package app

import (
	"context"

	"gorm.io/gorm"

	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/domain"
)

// CreateTemplate adds a template to the caller's tenant; the sources must
// parse and dry-run against the documented field set.
func (s *Service) CreateTemplate(ctx context.Context, params domain.NewTemplateParams) (domain.Template, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermCreate)
	if err != nil {
		return domain.Template{}, err
	}
	params.TenantID = p.TenantID
	params.CreatedBy = p.UserID
	t, err := domain.NewTemplate(params, s.now(), s.opts.RenderTimeout)
	if err != nil {
		return domain.Template{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateTemplate(ctx, tx, t)
	}); err != nil {
		return domain.Template{}, err
	}
	return t, nil
}

// GetTemplate returns a live template of the caller's tenant.
func (s *Service) GetTemplate(ctx context.Context, templateID string) (domain.Template, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermView)
	if err != nil {
		return domain.Template{}, err
	}
	return s.repo.TemplateByID(ctx, p.TenantID, templateID)
}

// ListTemplates pages templates newest first.
func (s *Service) ListTemplates(ctx context.Context, f TemplateFilter) ([]domain.Template, bool, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermViewAny)
	if err != nil {
		return nil, false, err
	}
	limit := clampLimit(f.Limit)
	f.Limit = limit + 1
	rows, err := s.repo.ListTemplates(ctx, p.TenantID, f)
	if err != nil {
		return nil, false, err
	}
	if len(rows) > limit {
		return rows[:limit], true, nil
	}
	return rows, false, nil
}

// UpdateTemplate applies a partial update under a row lock + version.
func (s *Service) UpdateTemplate(ctx context.Context, templateID string, patch domain.Patch) (domain.Template, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermUpdate)
	if err != nil {
		return domain.Template{}, err
	}
	var out domain.Template
	err = s.tx(ctx, func(tx *gorm.DB) error {
		t, err := s.repo.TemplateByIDForUpdate(ctx, tx, p.TenantID, templateID)
		if err != nil {
			return err
		}
		if err := t.Apply(patch, s.now(), s.opts.RenderTimeout); err != nil {
			return err
		}
		if err := s.repo.SaveTemplate(ctx, tx, t); err != nil {
			return err
		}
		t.Version++
		out = t
		return nil
	})
	if err != nil {
		return domain.Template{}, err
	}
	return out, nil
}

// DeleteTemplate soft-deletes; its history stays.
func (s *Service) DeleteTemplate(ctx context.Context, templateID string) error {
	p, err := s.requireTenantPerm(ctx, contracts.PermDelete)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		t, err := s.repo.TemplateByIDForUpdate(ctx, tx, p.TenantID, templateID)
		if err != nil {
			return err
		}
		now := s.now()
		t.DeletedAt = &now
		t.UpdatedAt = now
		return s.repo.SaveTemplate(ctx, tx, t)
	})
}

// Preview is a rendered template: plain title/body and the email HTML.
type Preview struct {
	Rendered domain.Rendered
	HTML     string
}

// PreviewTemplate renders a template against sample data; with a playerID
// the player fields are the real player's.
func (s *Service) PreviewTemplate(ctx context.Context, templateID, playerID string) (Preview, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermView)
	if err != nil {
		return Preview{}, err
	}
	t, err := s.repo.TemplateByID(ctx, p.TenantID, templateID)
	if err != nil {
		return Preview{}, err
	}
	data := domain.SampleData(t.Trigger)
	if playerID != "" {
		pl, err := s.requirePlayer(ctx, p.TenantID, playerID)
		if err != nil {
			return Preview{}, err
		}
		data.Player = domain.PlayerData{ID: pl.ID, ExternalID: pl.ExternalID, DisplayName: pl.DisplayName}
	}
	r, err := domain.Render(t, data, s.opts.RenderTimeout)
	if err != nil {
		return Preview{}, domain.ErrTemplateInvalid
	}
	return Preview{Rendered: r, HTML: domain.EmailHTML(r)}, nil
}

// GetChannels returns the tenant's channel settings (defaults when unsaved).
func (s *Service) GetChannels(ctx context.Context) (domain.ChannelSettings, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermViewAny)
	if err != nil {
		return domain.ChannelSettings{}, err
	}
	return s.channelSettings(ctx, p.TenantID)
}

// ChannelsPatch is a partial channel settings update.
type ChannelsPatch struct {
	EmailEnabled  *bool
	EmailFromName *string
}

// UpdateChannels upserts the tenant's channel settings.
func (s *Service) UpdateChannels(ctx context.Context, patch ChannelsPatch) (domain.ChannelSettings, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermChannelsManage)
	if err != nil {
		return domain.ChannelSettings{}, err
	}
	cs, err := s.channelSettings(ctx, p.TenantID)
	if err != nil {
		return domain.ChannelSettings{}, err
	}
	if patch.EmailEnabled != nil {
		cs.EmailEnabled = *patch.EmailEnabled
	}
	if patch.EmailFromName != nil {
		cs.EmailFromName = *patch.EmailFromName
	}
	cs.UpdatedAt = s.now()
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.UpsertChannelSettings(ctx, tx, cs)
	}); err != nil {
		return domain.ChannelSettings{}, err
	}
	return cs, nil
}
