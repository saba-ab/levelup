package eval

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
	_ "time/tzdata" // schedule timezones must not depend on the host's zoneinfo
)

// Schedule restricts when a rule may fire, judged on activity.occurred_at.
// A nil *Schedule always contains every instant.
type Schedule struct {
	StartsAt *time.Time // inclusive
	EndsAt   *time.Time // exclusive
	// Days is a bitmask of allowed local weekdays (bit 0 = Sunday); 0 means
	// every day.
	Days uint8
	// HasHours enables the [From, To) daily window, in minutes since local
	// midnight; From > To wraps midnight.
	HasHours bool
	From     int
	To       int
	Location *time.Location
}

// Contains reports whether t falls inside the schedule. A zero t (unknown
// occurrence time) is outside every non-nil schedule.
func (s *Schedule) Contains(t time.Time) bool {
	if s == nil {
		return true
	}
	if t.IsZero() {
		return false
	}
	if s.StartsAt != nil && t.Before(*s.StartsAt) {
		return false
	}
	if s.EndsAt != nil && !t.Before(*s.EndsAt) {
		return false
	}
	local := t.In(s.Location)
	if s.Days != 0 && s.Days&(1<<uint(local.Weekday())) == 0 {
		return false
	}
	if s.HasHours {
		m := local.Hour()*60 + local.Minute()
		if s.From < s.To {
			return m >= s.From && m < s.To
		}
		return m >= s.From || m < s.To
	}
	return true
}

func compileSchedule(raw json.RawMessage, fe fieldErrors) *Schedule {
	v, present, err := decode(raw)
	if err != nil {
		fe.add("schedule", "is not valid JSON")
		return nil
	}
	if !present || v == nil {
		return nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		fe.add("schedule", "must be an object")
		return nil
	}
	s := &Schedule{Location: time.UTC}
	bad := false
	for k, x := range obj {
		p := "schedule." + k
		switch k {
		case "starts_at", "ends_at":
			if x == nil {
				continue
			}
			str, isStr := x.(string)
			t, perr := time.Parse(time.RFC3339, str)
			if !isStr || perr != nil {
				fe.add(p, "must be an RFC 3339 timestamp")
				bad = true
				continue
			}
			t = t.UTC()
			if k == "starts_at" {
				s.StartsAt = &t
			} else {
				s.EndsAt = &t
			}
		case "days_of_week":
			if x == nil {
				continue
			}
			days, ok := compileDays(x, p, fe)
			if !ok {
				bad = true
				continue
			}
			s.Days = days
		case "hours":
			if x == nil {
				continue
			}
			if !compileHours(x, p, s, fe) {
				bad = true
			}
		case "timezone":
			if x == nil {
				continue
			}
			name, isStr := x.(string)
			if !isStr || name == "" || len(name) > MaxTimezone || name == "Local" {
				fe.add(p, "must be an IANA timezone name")
				bad = true
				continue
			}
			loc, lerr := time.LoadLocation(name)
			if lerr != nil {
				fe.add(p, fmt.Sprintf("unknown timezone %q", name))
				bad = true
				continue
			}
			s.Location = loc
		default:
			fe.add(p, "unknown key")
			bad = true
		}
	}
	if s.StartsAt != nil && s.EndsAt != nil && !s.EndsAt.After(*s.StartsAt) {
		fe.add("schedule.ends_at", "must be after starts_at")
		bad = true
	}
	if bad {
		return nil
	}
	return s
}

func compileDays(x any, path string, fe fieldErrors) (uint8, bool) {
	items, ok := x.([]any)
	if !ok || len(items) == 0 {
		fe.add(path, "must be a non-empty list of weekdays 0-6 (0 = Sunday)")
		return 0, false
	}
	var mask uint8
	for i, it := range items {
		n, isNum := it.(json.Number)
		d, err := strconv.Atoi(string(n))
		if !isNum || err != nil || d < 0 || d > 6 {
			fe.add(fmt.Sprintf("%s[%d]", path, i), "must be an integer 0-6 (0 = Sunday)")
			return 0, false
		}
		mask |= 1 << uint(d)
	}
	return mask, true
}

func compileHours(x any, path string, s *Schedule, fe fieldErrors) bool {
	obj, ok := x.(map[string]any)
	if !ok {
		fe.add(path, `must be an object {"from": "HH:MM", "to": "HH:MM"}`)
		return false
	}
	for k := range obj {
		if k != "from" && k != "to" {
			fe.add(path+"."+k, "unknown key")
			return false
		}
	}
	from, ok1 := clockMinutes(obj["from"])
	to, ok2 := clockMinutes(obj["to"])
	if !ok1 {
		fe.add(path+".from", "must be a time HH:MM (00:00-23:59)")
	}
	if !ok2 {
		fe.add(path+".to", "must be a time HH:MM (00:00-23:59)")
	}
	if !ok1 || !ok2 {
		return false
	}
	if from == to {
		fe.add(path+".to", "must differ from from")
		return false
	}
	s.HasHours, s.From, s.To = true, from, to
	return true
}

// clockMinutes parses "HH:MM" into minutes since midnight.
func clockMinutes(x any) (int, bool) {
	str, ok := x.(string)
	if !ok || len(str) != 5 || str[2] != ':' {
		return 0, false
	}
	for _, i := range []int{0, 1, 3, 4} {
		if str[i] < '0' || str[i] > '9' {
			return 0, false
		}
	}
	h, err1 := strconv.Atoi(str[:2])
	m, err2 := strconv.Atoi(str[3:])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}
