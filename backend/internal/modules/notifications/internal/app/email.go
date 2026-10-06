package app

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/domain"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/mail"
	"levelup/internal/shared/errs"
)

// HandleSendEmail consumes job.notifications.send_email.
//
//   - Malformed payload → errs.Invalid (immediate DLQ).
//   - Row gone (tenant purged) or already settled → ack (idempotent).
//   - Row leased by a concurrent delivery → errs.Unavailable: the ladder
//     retries and the retry finds it settled.
//   - Email disabled since fan-out, player gone or without email → skipped.
//   - Mailer errs.Invalid (recipient rejected) → failed, ack.
//   - Any other mailer error → transient: the row stays pending with
//     last_error and the job returns errs.Unavailable to climb the retry
//     ladder, until EmailMaxAttempts settles it as failed.
func (s *Service) HandleSendEmail(ctx context.Context, body []byte) error {
	var cmd contracts.SendEmailCmdV1
	if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
		return err
	}
	fields := map[string]string{}
	if _, err := uuid.Parse(cmd.TenantID); err != nil {
		fields["tenant_id"] = "must be a valid UUID"
	}
	if _, err := uuid.Parse(cmd.NotificationID); err != nil {
		fields["notification_id"] = "must be a valid UUID"
	}
	if len(fields) > 0 {
		return errs.WithFields(errs.New(errs.Invalid, "malformed send_email command"), fields)
	}
	return s.sendEmail(ctx, cmd.TenantID, cmd.NotificationID)
}

func (s *Service) sendEmail(ctx context.Context, tenantID, notificationID string) error {
	n, found, err := s.repo.NotificationByID(ctx, tenantID, notificationID)
	if err != nil {
		return err
	}
	if !found || n.Channel != contracts.ChannelEmail || !n.Pending() {
		return nil
	}

	settings, err := s.channelSettings(ctx, tenantID)
	if err != nil {
		return err
	}
	if !settings.EmailEnabled {
		return s.settle(ctx, n, func(n *domain.Notification) { n.Skip(contracts.ReasonEmailDisabled, s.now()) })
	}
	player, found, err := s.players.ByID(ctx, tenantID, n.PlayerID)
	if err != nil {
		return err
	}
	if !found || player.TenantID != tenantID {
		return s.settle(ctx, n, func(n *domain.Notification) { n.Skip(contracts.ReasonPlayerNotFound, s.now()) })
	}
	to := strings.TrimSpace(player.Email)
	if to == "" {
		return s.settle(ctx, n, func(n *domain.Notification) { n.Skip(contracts.ReasonNoEmail, s.now()) })
	}

	// Claim the lease in its own transaction, then call the mailer outside
	// any transaction, then settle (CLAUDE.md: never call out inside a tx).
	now := s.now()
	var claimed bool
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		var cerr error
		n, claimed, cerr = s.repo.ClaimEmail(ctx, tx, tenantID, notificationID, now, now.Add(s.opts.SendTimeout+s.opts.SendTimeout/2))
		return cerr
	}); err != nil {
		return err
	}
	if !claimed {
		cur, found, err := s.repo.NotificationByID(ctx, tenantID, notificationID)
		if err != nil {
			return err
		}
		if found && cur.Pending() {
			return errs.New(errs.Unavailable, "email notification is being sent by another worker")
		}
		return nil
	}

	r := domain.Rendered{Title: n.Title, Body: n.Body}
	sendCtx, cancel := context.WithTimeout(ctx, s.opts.SendTimeout)
	sendErr := s.mailer.Send(sendCtx, mail.Message{To: to, Subject: r.Title, Text: r.Body, HTML: domain.EmailHTML(r), FromName: settings.EmailFromName})
	cancel()

	switch {
	case sendErr == nil:
		return s.settle(ctx, n, func(n *domain.Notification) { n.Deliver(s.now()) })
	case errs.KindOf(sendErr) == errs.Invalid:
		s.log.Warn("email notification rejected", zap.String("notification_id", n.ID), zap.Error(sendErr))
		return s.settle(ctx, n, func(n *domain.Notification) { n.Fail(contracts.ReasonMailRejected, sendErr.Error(), s.now()) })
	case n.Attempts >= s.opts.EmailMaxAttempts:
		s.log.Warn("email notification failed after retries", zap.String("notification_id", n.ID),
			zap.Int("attempts", n.Attempts), zap.Error(sendErr))
		return s.settle(ctx, n, func(n *domain.Notification) {
			n.Fail(contracts.ReasonRetriesExhaust, sendErr.Error(), s.now())
		})
	default:
		if err := s.settle(ctx, n, func(n *domain.Notification) { n.Retry(sendErr.Error(), s.now()) }); err != nil {
			return err
		}
		return errs.Wrap(errs.Unavailable, "send email notification", sendErr)
	}
}

// settle applies mut and writes the row back while it is still pending.
func (s *Service) settle(ctx context.Context, n domain.Notification, mut func(*domain.Notification)) error {
	mut(&n)
	return s.tx(ctx, func(tx *gorm.DB) error {
		_, err := s.repo.SettleNotification(ctx, tx, n)
		return err
	})
}

// SweepStaleEmail fails email rows still pending EmailStaleAfter after
// creation: their job was parked in the DLQ or lost with a crashed worker.
// Reconciling: it looks at every pending row older than the cutoff, so a
// missed run is caught up by the next one.
func (s *Service) SweepStaleEmail(ctx context.Context) error {
	now := s.now()
	var n int64
	err := s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		n, err = s.repo.FailStalePending(ctx, tx, now.Add(-s.opts.EmailStaleAfter), now)
		return err
	})
	if err != nil {
		return err
	}
	if n > 0 {
		s.log.Warn("stale email notifications failed", zap.Int64("count", n))
	}
	return nil
}
