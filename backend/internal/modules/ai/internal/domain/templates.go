package domain

import "levelup/internal/modules/ai/contracts"

// Template is one entry of the AI Hub's static prompt catalogue.
type Template struct {
	ID            string
	Name          string
	Kind          string
	Description   string
	ExamplePrompt string
	DefaultCount  int
}

// Templates is the AI Hub catalogue, in display order.
var Templates = []Template{
	{
		ID: "badge-concept", Name: "Badge Concept", Kind: contracts.KindBadge, DefaultCount: 1,
		Description:   "One badge with a name, tier, category and unlock criteria from a short idea.",
		ExamplePrompt: "A badge for customers who leave their first product review.",
	},
	{
		ID: "badge-collection", Name: "Badge Collection", Kind: contracts.KindBadge, DefaultCount: 5,
		Description:   "A themed set of badges that escalate from bronze to diamond.",
		ExamplePrompt: "A five-badge collection for a fitness app rewarding workout consistency.",
	},
	{
		ID: "level-description", Name: "Level Description", Kind: contracts.KindLevel, DefaultCount: 1,
		Description:   "Name, description and XP threshold for the next rung of your ladder.",
		ExamplePrompt: "The next level after our current top level, themed around mastery.",
	},
	{
		ID: "tier-benefits", Name: "Tier Benefits", Kind: contracts.KindLevel, DefaultCount: 4,
		Description:   "A run of levels with escalating XP thresholds and member benefits.",
		ExamplePrompt: "Four loyalty tiers for a coffee chain, with perks like free upsizes and early access.",
	},
	{
		ID: "onboarding-mission", Name: "Onboarding Mission", Kind: contracts.KindMission, DefaultCount: 3,
		Description:   "First-week missions that walk new players through the core actions.",
		ExamplePrompt: "Onboarding missions for new users: complete profile, first purchase, invite a friend.",
	},
	{
		ID: "reward-ideas", Name: "Reward Ideas", Kind: contracts.KindReward, DefaultCount: 5,
		Description:   "Catalogue rewards with point costs that fit your economy.",
		ExamplePrompt: "Rewards for an online bookstore, from a 5% discount to a signed first edition.",
	},
	{
		ID: "point-rules", Name: "Point Rules", Kind: contracts.KindRule, DefaultCount: 3,
		Description:   "Rules that credit points or XP when players perform tracked events.",
		ExamplePrompt: "Give 10 points per purchase, double for orders over 100, at most 5 times a day.",
	},
	{
		ID: "automation-rules", Name: "Automation Rules", Kind: contracts.KindRule, DefaultCount: 3,
		Description:   "Rules that award badges, progress missions or grant rewards automatically.",
		ExamplePrompt: "Award the first-review badge when a player submits their first review.",
	},
	{
		ID: "segment-definition", Name: "Segment Definition", Kind: contracts.KindSegment, DefaultCount: 1,
		Description:   "One player segment expressed as conditions on player fields.",
		ExamplePrompt: "Players above level 5 who live in Germany or Austria.",
	},
	{
		ID: "segmentation-strategy", Name: "Segmentation Strategy", Kind: contracts.KindSegment, DefaultCount: 4,
		Description:   "A set of complementary segments for targeting campaigns.",
		ExamplePrompt: "Segments for a re-engagement campaign: new, active, at-risk and lapsed players.",
	},
}
