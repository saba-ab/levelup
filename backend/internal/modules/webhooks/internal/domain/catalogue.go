package domain

import (
	"slices"
	"strings"

	activitycontracts "levelup/internal/modules/activity/contracts"
	badgescontracts "levelup/internal/modules/badges/contracts"
	leaderboardscontracts "levelup/internal/modules/leaderboards/contracts"
	missionscontracts "levelup/internal/modules/missions/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	rewardscontracts "levelup/internal/modules/rewards/contracts"
	rulescontracts "levelup/internal/modules/rules/contracts"
	streakscontracts "levelup/internal/modules/streaks/contracts"
)

// Wildcard subscribes an endpoint to every catalogue event.
const Wildcard = "*"

// TestEvent is the synthetic event POST /webhooks/{id}/test sends. It is not
// subscribable.
const TestEvent = "webhook.test"

// EventType is one deliverable fact: the bus topic it is fed from and the
// webhook event name (the topic without its ".v1" version suffix).
type EventType struct {
	Topic       string
	Event       string
	Description string
}

// Catalogue is the fixed list of facts delivered to webhooks, in display
// order.
var Catalogue = []EventType{
	ev(playercontracts.TopicPlayerCreated, "A player was created."),
	ev(playercontracts.TopicPlayerUpdated, "A player's profile or status changed."),
	ev(pointscontracts.TopicCredited, "Points were credited to a player's wallet."),
	ev(pointscontracts.TopicDebited, "Points were debited from a player's wallet."),
	ev(badgescontracts.TopicAwarded, "A badge was awarded to a player."),
	ev(progressioncontracts.TopicLevelReached, "A player reached a new level."),
	ev(missionscontracts.TopicCompleted, "A player completed a mission."),
	ev(streakscontracts.TopicMilestoneReached, "A player's streak reached a milestone."),
	ev(streakscontracts.TopicBroken, "A player's streak was broken."),
	ev(rewardscontracts.TopicClaimed, "A player claimed a reward."),
	ev(rewardscontracts.TopicRedeemed, "A claimed reward was redeemed."),
	ev(rulescontracts.TopicDecisionMade, "The rules engine decided on an activity."),
	ev(leaderboardscontracts.TopicPeriodClosed, "A leaderboard period closed."),
	ev(activitycontracts.TopicReceived, "An activity was received."),
}

func ev(topic, description string) EventType {
	return EventType{Topic: topic, Event: EventName(topic), Description: description}
}

// EventName drops the version suffix: "badges.awarded.v1" → "badges.awarded".
func EventName(topic string) string {
	i := strings.LastIndex(topic, ".v")
	if i < 0 {
		return topic
	}
	return topic[:i]
}

// EventForTopic returns the webhook event name of a catalogue topic.
func EventForTopic(topic string) (string, bool) {
	for _, e := range Catalogue {
		if e.Topic == topic {
			return e.Event, true
		}
	}
	return "", false
}

// IsKnownEvent reports whether name is a catalogue event name.
func IsKnownEvent(name string) bool {
	return slices.ContainsFunc(Catalogue, func(e EventType) bool { return e.Event == name })
}

// NormalizeEventTypes validates a subscription list: a non-empty subset of
// the catalogue's event names, or exactly ["*"]. Duplicates collapse; the
// result is sorted.
func NormalizeEventTypes(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, ErrEventTypesRequired
	}
	if len(in) > 2*len(Catalogue) {
		return nil, ErrTooManyEventTypes
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		name := strings.TrimSpace(raw)
		if name == Wildcard {
			seen[Wildcard] = true
			continue
		}
		if !IsKnownEvent(name) {
			return nil, UnknownEventType(name)
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	if seen[Wildcard] {
		if len(out) > 0 {
			return nil, ErrWildcardNotAlone
		}
		return []string{Wildcard}, nil
	}
	slices.Sort(out)
	return out, nil
}

// Subscribes reports whether a subscription list covers event.
func Subscribes(eventTypes []string, event string) bool {
	return slices.Contains(eventTypes, Wildcard) || slices.Contains(eventTypes, event)
}
