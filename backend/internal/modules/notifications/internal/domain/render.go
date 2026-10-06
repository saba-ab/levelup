package domain

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	"levelup/internal/modules/notifications/contracts"
)

// Data is the documented, closed field set a template can read. A field that
// is not here fails validation on save ({{.Player.Email}} is deliberately
// absent). Fields a trigger does not populate render as their zero value.
//
//	{{.Trigger}}
//	{{.Player.ID}} {{.Player.ExternalID}} {{.Player.DisplayName}}
//	{{.Badge.ID}} {{.Badge.Slug}} {{.Badge.Name}} {{.Badge.Tier}} {{.Badge.Category}} {{.Badge.EarnedCount}} {{.Badge.PointsValue}}
//	{{.Level.ID}} {{.Level.Number}} {{.Level.Name}} {{.Level.TotalXP}} {{.Level.PointsReward}}
//	{{.Points.Amount}} {{.Points.Balance}} {{.Points.LifetimeEarned}} {{.Points.Kind}}
//	{{.Mission.ID}} {{.Mission.Slug}} {{.Mission.Name}} {{.Mission.PointsReward}} {{.Mission.XPReward}}
//	{{.Streak.ID}} {{.Streak.Milestone}} {{.Streak.BonusPoints}} {{.Streak.FinalCount}}
//	{{.Reward.ID}} {{.Reward.Slug}} {{.Reward.Name}} {{.Reward.Type}} {{.Reward.Code}} {{.Reward.Value}} {{.Reward.PointsCost}}
type Data struct {
	Trigger string
	Player  PlayerData
	Badge   BadgeData
	Level   LevelData
	Points  PointsData
	Mission MissionData
	Streak  StreakData
	Reward  RewardData
}

type PlayerData struct {
	ID          string
	ExternalID  string
	DisplayName string
}

// BadgeData.Name is the badge slug: badges.awarded.v1 carries no display name.
type BadgeData struct {
	ID          string
	Slug        string
	Name        string
	Tier        string
	Category    string
	EarnedCount int
	PointsValue int64
}

type LevelData struct {
	ID           string
	Number       int
	Name         string
	TotalXP      int64
	PointsReward int64
}

type PointsData struct {
	Amount         int64
	Balance        int64
	LifetimeEarned int64
	Kind           string
}

// MissionData.Name is the mission slug: missions.completed.v1 carries no name.
type MissionData struct {
	ID           string
	Slug         string
	Name         string
	PointsReward int64
	XPReward     int64
}

type StreakData struct {
	ID          string
	Milestone   int
	BonusPoints int64
	FinalCount  int
}

// RewardData.Name is the reward slug: rewards.claimed.v1 carries no name.
type RewardData struct {
	ID         string
	Slug       string
	Name       string
	Type       string
	Code       string
	Value      string
	PointsCost int64
}

// SampleData is what validation dry-runs a template against and what a
// preview shows when no player is given.
func SampleData(trigger string) Data {
	return Data{
		Trigger: trigger,
		Player:  PlayerData{ID: "00000000-0000-0000-0000-000000000001", ExternalID: "player-42", DisplayName: "Alex"},
		Badge: BadgeData{ID: "00000000-0000-0000-0000-0000000000b1", Slug: "first-steps", Name: "first-steps",
			Tier: "gold", Category: "achievement", EarnedCount: 1, PointsValue: 100},
		Level:   LevelData{ID: "00000000-0000-0000-0000-0000000000c1", Number: 5, Name: "Explorer", TotalXP: 1200, PointsReward: 50},
		Points:  PointsData{Amount: 250, Balance: 1250, LifetimeEarned: 4000, Kind: "earn"},
		Mission: MissionData{ID: "00000000-0000-0000-0000-0000000000d1", Slug: "weekly-quest", Name: "weekly-quest", PointsReward: 300, XPReward: 150},
		Streak:  StreakData{ID: "00000000-0000-0000-0000-0000000000e1", Milestone: 7, BonusPoints: 70, FinalCount: 12},
		Reward: RewardData{ID: "00000000-0000-0000-0000-0000000000f1", Slug: "free-coffee", Name: "free-coffee",
			Type: "coupon", Code: "COFFEE-1234", Value: "1", PointsCost: 500},
	}
}

// Limits on template sources and rendered output.
const (
	MaxTitleSource  = 500
	MaxBodySource   = 10000
	MaxTitleOutput  = 1000
	MaxRenderOutput = 64 << 10
)

// Compiled is a parsed, safety-checked template.
type Compiled struct{ t *template.Template }

// Compile parses src with missingkey=zero and refuses constructs that could
// make execution unbounded or reach outside the data: {{define}}, {{block}},
// {{template}} and {{range}} (Data has no collections, and range over an
// integer would loop). Execution is linear in the source size afterwards.
func Compile(src string) (*Compiled, error) {
	t, err := template.New("n").Option("missingkey=zero").Parse(src)
	if err != nil {
		return nil, err
	}
	if len(t.Templates()) > 1 {
		return nil, errors.New("{{define}} and {{block}} are not allowed")
	}
	if t.Tree != nil {
		if err := checkNodes(t.Root); err != nil {
			return nil, err
		}
	}
	return &Compiled{t: t}, nil
}

func checkNodes(n parse.Node) error {
	switch v := n.(type) {
	case nil:
		return nil
	case *parse.ListNode:
		if v == nil {
			return nil
		}
		for _, c := range v.Nodes {
			if err := checkNodes(c); err != nil {
				return err
			}
		}
	case *parse.RangeNode:
		return errors.New("{{range}} is not allowed")
	case *parse.TemplateNode:
		return errors.New("{{template}} is not allowed")
	case *parse.ActionNode:
		return checkPipe(v.Pipe)
	case *parse.IfNode:
		return checkBranch(&v.BranchNode)
	case *parse.WithNode:
		return checkBranch(&v.BranchNode)
	}
	return nil
}

// allowedFuncs are the builtins a template may call. printf (unbounded
// width allocations), call, index, slice and js are refused.
var allowedFuncs = map[string]bool{
	"and": true, "or": true, "not": true, "eq": true, "ne": true, "lt": true, "le": true,
	"gt": true, "ge": true, "len": true, "html": true, "urlquery": true, "print": true,
}

func checkPipe(p *parse.PipeNode) error {
	if p == nil {
		return nil
	}
	for _, cmd := range p.Cmds {
		for _, arg := range cmd.Args {
			switch a := arg.(type) {
			case *parse.IdentifierNode:
				if !allowedFuncs[a.Ident] {
					return fmt.Errorf("function %q is not allowed", a.Ident)
				}
			case *parse.PipeNode:
				if err := checkPipe(a); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkBranch(b *parse.BranchNode) error {
	if err := checkPipe(b.Pipe); err != nil {
		return err
	}
	if err := checkNodes(b.List); err != nil {
		return err
	}
	if b.ElseList != nil {
		return checkNodes(b.ElseList)
	}
	return nil
}

// limitWriter fails the execution once max bytes were written.
type limitWriter struct {
	buf bytes.Buffer
	max int
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		return 0, ErrRenderTooLarge
	}
	return w.buf.Write(p)
}

// Execute renders with a deadline and an output cap. A panic inside the
// template engine is turned into an error.
func (c *Compiled) Execute(data Data, timeout time.Duration, maxOut int) (string, error) {
	if timeout <= 0 {
		timeout = 250 * time.Millisecond
	}
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("template panicked: %v", r)}
			}
		}()
		w := &limitWriter{max: maxOut}
		err := c.t.Execute(w, data)
		done <- result{out: w.buf.String(), err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		if r.err != nil {
			if errors.Is(r.err, ErrRenderTooLarge) {
				return "", ErrRenderTooLarge
			}
			return "", r.err
		}
		return r.out, nil
	case <-timer.C:
		return "", ErrRenderTimeout
	}
}

// Rendered is a notification's title and plain-text body.
type Rendered struct {
	Title string
	Body  string
}

// Render executes a template's title and body. The title is collapsed to a
// single line (it becomes the email Subject header).
func Render(t Template, data Data, timeout time.Duration) (Rendered, error) {
	title, err := Compile(t.TitleTemplate)
	if err != nil {
		return Rendered{}, err
	}
	body, err := Compile(t.BodyTemplate)
	if err != nil {
		return Rendered{}, err
	}
	ti, err := title.Execute(data, timeout, MaxTitleOutput)
	if err != nil {
		return Rendered{}, err
	}
	bo, err := body.Execute(data, timeout, MaxRenderOutput)
	if err != nil {
		return Rendered{}, err
	}
	return Rendered{Title: singleLine(ti), Body: bo}, nil
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// EmailHTML is the HTML part of an email: the rendered text is HTML-escaped
// (templates are plain text; nothing a player controls, such as a display
// name, can inject markup) and line breaks become <br>.
func EmailHTML(r Rendered) string {
	var b strings.Builder
	b.WriteString("<!doctype html><html><body>")
	b.WriteString("<h2>")
	b.WriteString(html.EscapeString(r.Title))
	b.WriteString("</h2>")
	for i, para := range strings.Split(strings.ReplaceAll(r.Body, "\r\n", "\n"), "\n\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(html.EscapeString(para), "\n", "<br>"))
		b.WriteString("</p>")
	}
	b.WriteString("</body></html>")
	return b.String()
}

// IsTrigger reports whether s is a known trigger.
func IsTrigger(s string) bool {
	for _, t := range contracts.AllTriggers {
		if t == s {
			return true
		}
	}
	return false
}
