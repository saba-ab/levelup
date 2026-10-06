package app

import (
	"fmt"

	"levelup/internal/config"
	"levelup/internal/modules/activity"
	activityadapters "levelup/internal/modules/activity/adapters"
	"levelup/internal/modules/ai"
	"levelup/internal/modules/analytics"
	"levelup/internal/modules/badges"
	badgesadapters "levelup/internal/modules/badges/adapters"
	"levelup/internal/modules/eventcatalog"
	"levelup/internal/modules/identity"
	"levelup/internal/modules/leaderboards"
	leaderboardsadapters "levelup/internal/modules/leaderboards/adapters"
	"levelup/internal/modules/missions"
	missionsadapters "levelup/internal/modules/missions/adapters"
	"levelup/internal/modules/notifications"
	notifadapters "levelup/internal/modules/notifications/adapters"
	"levelup/internal/modules/player"
	"levelup/internal/modules/points"
	pointsadapters "levelup/internal/modules/points/adapters"
	"levelup/internal/modules/program"
	programadapters "levelup/internal/modules/program/adapters"
	"levelup/internal/modules/progression"
	progressionadapters "levelup/internal/modules/progression/adapters"
	"levelup/internal/modules/rewards"
	rewardsadapters "levelup/internal/modules/rewards/adapters"
	"levelup/internal/modules/rules"
	rulesadapters "levelup/internal/modules/rules/adapters"
	"levelup/internal/modules/segments"
	segmentsadapters "levelup/internal/modules/segments/adapters"
	"levelup/internal/modules/streaks"
	streaksadapters "levelup/internal/modules/streaks/adapters"
	"levelup/internal/modules/webhooks"
	"levelup/internal/platform/mail"
	"levelup/internal/platform/modkit"
)

// buildModules is the ONE place modules are listed (PRD §6). Selection is
// runtime via MODULES_ENABLED (R2), in dependency order: a consumer listed
// before its provider fails the boot naming both.
//
// Every edge between modules is visible here: a consumer receives a local
// adapter over the provider's contracts.Reader. Swapping one for a remote
// adapter is the extraction seam (docs/examples.md §10).
func buildModules(p *Platform, cfg config.Config) ([]modkit.Module, error) {
	mailer, err := mail.New(mail.Config{
		Driver: cfg.Mail.Driver, From: cfg.Mail.From, Host: cfg.Mail.Host, Port: cfg.Mail.Port,
		Username: cfg.Mail.Username, Password: cfg.Mail.Password,
	}, p.Tel.Log)
	if err != nil {
		return nil, err
	}
	var (
		identityMod     *identity.Module
		playerMod       *player.Module
		eventcatalogMod *eventcatalog.Module
		programMod      *program.Module
		pointsMod       *points.Module
		progressionMod  *progression.Module
		badgesMod       *badges.Module
		activityMod     *activity.Module
	)

	all := map[string]func() modkit.Module{
		"identity": func() modkit.Module {
			identityMod = identity.New(p.DepsFor("identity"), cfg.Identity, p.Authn, mailer)
			return identityMod
		},
		"player": func() modkit.Module {
			playerMod = player.New(p.DepsFor("player"), cfg.Player)
			return playerMod
		},
		"eventcatalog": func() modkit.Module {
			eventcatalogMod = eventcatalog.New(p.DepsFor("eventcatalog"), cfg.EventCatalog)
			return eventcatalogMod
		},
		"program": func() modkit.Module {
			need(playerMod, "program", "player")
			programMod = program.New(p.DepsFor("program"), cfg.Program,
				programadapters.NewLocalPlayers(playerMod.Reader()))
			return programMod
		},
		"points": func() modkit.Module {
			need(playerMod, "points", "player")
			pointsMod = points.New(p.DepsFor("points"), cfg.Points,
				pointsadapters.NewLocalPlayers(playerMod.Reader()))
			return pointsMod
		},
		"badges": func() modkit.Module {
			need(playerMod, "badges", "player")
			badgesMod = badges.New(p.DepsFor("badges"), cfg.Badges,
				badgesadapters.NewLocalPlayers(playerMod.Reader()))
			return badgesMod
		},
		"progression": func() modkit.Module {
			need(playerMod, "progression", "player")
			progressionMod = progression.New(p.DepsFor("progression"), cfg.Progression,
				progressionadapters.NewLocalPlayers(playerMod.Reader()))
			return progressionMod
		},
		"streaks": func() modkit.Module {
			need(playerMod, "streaks", "player")
			need(identityMod, "streaks", "identity")
			return streaks.New(p.DepsFor("streaks"), cfg.Streaks,
				streaksadapters.NewLocalPlayers(playerMod.Reader()),
				streaksadapters.NewLocalTenants(identityMod.TenantReader()))
		},
		"missions": func() modkit.Module {
			need(playerMod, "missions", "player")
			return missions.New(p.DepsFor("missions"), cfg.Missions,
				missionsadapters.NewLocalPlayers(playerMod.Reader()))
		},
		"rewards": func() modkit.Module {
			need(playerMod, "rewards", "player")
			need(progressionMod, "rewards", "progression")
			need(pointsMod, "rewards", "points")
			return rewards.New(p.DepsFor("rewards"), cfg.Rewards,
				rewardsadapters.NewLocalPlayers(playerMod.Reader()),
				rewardsadapters.NewLocalProgress(progressionMod.Reader()),
				rewardsadapters.NewLocalPoints(pointsMod.Reader()))
		},
		"leaderboards": func() modkit.Module {
			need(playerMod, "leaderboards", "player")
			return leaderboards.New(p.DepsFor("leaderboards"), cfg.Leaderboards,
				leaderboardsadapters.NewLocalPlayers(playerMod.Reader()))
		},
		"activity": func() modkit.Module {
			need(playerMod, "activity", "player")
			need(eventcatalogMod, "activity", "eventcatalog")
			activityMod = activity.New(p.DepsFor("activity"), cfg.Activity,
				activityadapters.NewLocalPlayers(playerMod.Reader()),
				activityadapters.NewLocalEventTypes(eventcatalogMod.Reader()))
			return activityMod
		},
		"notifications": func() modkit.Module {
			need(playerMod, "notifications", "player")
			return notifications.New(p.DepsFor("notifications"), cfg.Notifications,
				notifadapters.NewLocalPlayers(playerMod.Reader()), mailer)
		},
		"segments": func() modkit.Module {
			need(playerMod, "segments", "player")
			need(progressionMod, "segments", "progression")
			need(pointsMod, "segments", "points")
			need(badgesMod, "segments", "badges")
			need(activityMod, "segments", "activity")
			return segments.New(p.DepsFor("segments"), cfg.Segments,
				segmentsadapters.NewLocalPlayers(playerMod.Reader(), playerMod.IDLister()),
				segmentsadapters.NewLocalProgress(progressionMod.Reader()),
				segmentsadapters.NewLocalWallets(pointsMod.Reader()),
				segmentsadapters.NewLocalBadges(badgesMod.Reader()),
				segmentsadapters.NewLocalActivity(activityMod.Reader()))
		},
		"analytics": func() modkit.Module {
			return analytics.New(p.DepsFor("analytics"), cfg.Analytics)
		},
		"webhooks": func() modkit.Module {
			return webhooks.New(p.DepsFor("webhooks"), cfg.Webhooks)
		},
		"ai": func() modkit.Module {
			return ai.New(p.DepsFor("ai"), cfg.AI)
		},
		"rules": func() modkit.Module {
			need(playerMod, "rules", "player")
			need(progressionMod, "rules", "progression")
			need(pointsMod, "rules", "points")
			m := rules.New(p.DepsFor("rules"), cfg.Rules,
				rulesadapters.NewLocalPlayers(playerMod.Reader()),
				rulesadapters.NewLocalProgress(progressionMod.Reader()),
				rulesadapters.NewLocalPoints(pointsMod.Reader()))
			// Program scoping is optional: without program, program-scoped
			// rules apply tenant-wide.
			if programMod != nil {
				m.WithPrograms(rulesadapters.NewLocalPrograms(programMod.Reader()))
			}
			return m
		},
	}
	return selectModules(all, cfg.ModulesEnabled)
}

// need fails the boot when a consumer is enabled before (or without) the
// module it reads from (R2): the message names the fix.
func need[M any](provider *M, consumer, providerName string) {
	if provider == nil {
		panic(fmt.Sprintf("%s needs the %s module: enable %q BEFORE %q in MODULES_ENABLED",
			consumer, providerName, providerName, consumer))
	}
}

// selectModules filters constructors by the enabled list, preserving the
// enabled order. Unknown names fail fast at boot, not at first use.
func selectModules(all map[string]func() modkit.Module, enabled []string) ([]modkit.Module, error) {
	out := make([]modkit.Module, 0, len(enabled))
	seen := map[string]bool{}
	for _, name := range enabled {
		if seen[name] {
			return nil, fmt.Errorf("module %q enabled twice", name)
		}
		seen[name] = true
		ctor, ok := all[name]
		if !ok {
			return nil, fmt.Errorf("unknown module %q in MODULES_ENABLED", name)
		}
		out = append(out, ctor())
	}
	return out, nil
}
