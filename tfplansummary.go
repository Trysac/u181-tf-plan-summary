package tfplansummary

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
)

// DefaultMaxBytes is the default maximum input size (50 MiB).
const DefaultMaxBytes int64 = 52428800

// Sentinel errors returned by the package.
var (
	// ErrTooLarge is returned when the plan input exceeds the maximum byte limit.
	ErrTooLarge = errors.New("tfplansummary: plan exceeds maximum size")

	// ErrInvalidPlan is returned when the plan JSON is malformed or contains invalid change structures.
	ErrInvalidPlan = errors.New("tfplansummary: invalid plan")

	// ErrUnsupportedFormat is returned when the plan format_version major version is not supported.
	ErrUnsupportedFormat = errors.New("tfplansummary: unsupported format_version")
)

// Action represents the Terraform lifecycle action applied to a resource.
type Action string

const (
	ActionCreate  Action = "create"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"
	ActionReplace Action = "replace"
)

// Level represents the assessed risk level of a change.
type Level int

const (
	LevelNone Level = iota
	LevelLow
	LevelMedium
	LevelHigh
	LevelCritical
)

// String returns the canonical lowercase name of the risk level.
func (l Level) String() string {
	switch l {
	case LevelNone:
		return "none"
	case LevelLow:
		return "low"
	case LevelMedium:
		return "medium"
	case LevelHigh:
		return "high"
	case LevelCritical:
		return "critical"
	default:
		return fmt.Sprintf("level(%d)", int(l))
	}
}

// MarshalJSON marshals the Level as its string representation.
func (l Level) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.String())
}

// UnmarshalJSON unmarshals a string representation into a Level.
func (l *Level) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := ParseLevel(s)
	if err != nil {
		return err
	}
	*l = parsed
	return nil
}

// ParseLevel parses a string into a Level.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none":
		return LevelNone, nil
	case "low":
		return LevelLow, nil
	case "medium":
		return LevelMedium, nil
	case "high":
		return LevelHigh, nil
	case "critical":
		return LevelCritical, nil
	default:
		return LevelNone, fmt.Errorf("unknown level %q", s)
	}
}

// Rule specifies custom risk level overrides matching resource types and actions.
type Rule struct {
	TypePattern string
	Actions     []Action
	Level       Level
}

// DefaultRules contains protected database, storage, and IAM resource rules.
var DefaultRules = []Rule{
	{
		TypePattern: "aws_db_instance",
		Actions:     []Action{ActionDelete, ActionReplace},
		Level:       LevelCritical,
	},
	{
		TypePattern: "aws_rds_cluster",
		Actions:     []Action{ActionDelete, ActionReplace},
		Level:       LevelCritical,
	},
	{
		TypePattern: "aws_s3_bucket",
		Actions:     []Action{ActionDelete, ActionReplace},
		Level:       LevelCritical,
	},
	{
		TypePattern: "aws_dynamodb_table",
		Actions:     []Action{ActionDelete, ActionReplace},
		Level:       LevelCritical,
	},
	{
		TypePattern: "google_sql_database_instance",
		Actions:     []Action{ActionDelete, ActionReplace},
		Level:       LevelCritical,
	},
	{
		TypePattern: "google_storage_bucket",
		Actions:     []Action{ActionDelete, ActionReplace},
		Level:       LevelCritical,
	},
	{
		TypePattern: "azurerm_mssql_database",
		Actions:     []Action{ActionDelete, ActionReplace},
		Level:       LevelCritical,
	},
	{
		TypePattern: "azurerm_storage_account",
		Actions:     []Action{ActionDelete, ActionReplace},
		Level:       LevelCritical,
	},
	{
		TypePattern: "*iam*",
		Actions:     nil,
		Level:       LevelHigh,
	},
}

// Options configures plan parsing limits and evaluation rules.
type Options struct {
	MaxBytes int64
	Rules    []Rule
}

// Change describes a planned change to a single resource.
type Change struct {
	Address    string   `json:"address"`
	Type       string   `json:"type"`
	Action     Action   `json:"action"`
	Level      Level    `json:"level"`
	Attributes []string `json:"attributes"`
}

// Summary aggregates planned changes, action counts, and the highest overall risk.
type Summary struct {
	Changes  []Change       `json:"changes"`
	Counts   map[Action]int `json:"counts"`
	MaxLevel Level          `json:"max_level"`
}

type rawPlan struct {
	FormatVersion   string              `json:"format_version"`
	ResourceChanges []rawResourceChange `json:"resource_changes"`
}

type rawResourceChange struct {
	Address string    `json:"address"`
	Mode    string    `json:"mode"`
	Type    string    `json:"type"`
	Change  rawChange `json:"change"`
}

type rawChange struct {
	Actions         []string `json:"actions"`
	Before          any      `json:"before"`
	After           any      `json:"after"`
	AfterUnknown    any      `json:"after_unknown"`
	BeforeSensitive any      `json:"before_sensitive"`
	AfterSensitive  any      `json:"after_sensitive"`
}

// Parse reads Terraform plan JSON from r according to opts.
func Parse(r io.Reader, opts Options) (*Summary, error) {
	max := opts.MaxBytes
	if max <= 0 {
		max = DefaultMaxBytes
	}

	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, fmt.Errorf("%w: error reading stream", ErrInvalidPlan)
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%w: input exceeds limit of %d bytes", ErrTooLarge, max)
	}

	var plan rawPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("%w: failed to decode json: %v", ErrInvalidPlan, err)
	}

	if plan.FormatVersion == "" {
		return nil, fmt.Errorf("%w: missing format_version", ErrInvalidPlan)
	}
	parts := strings.Split(plan.FormatVersion, ".")
	if len(parts) == 0 || parts[0] != "1" {
		return nil, fmt.Errorf("%w: format_version major version must be 1, got %q", ErrUnsupportedFormat, plan.FormatVersion)
	}

	rules := opts.Rules
	if rules == nil {
		rules = DefaultRules
	}

	summary := &Summary{
		Changes: make([]Change, 0),
		Counts: map[Action]int{
			ActionCreate:  0,
			ActionUpdate:  0,
			ActionDelete:  0,
			ActionReplace: 0,
		},
		MaxLevel: LevelNone,
	}

	for _, rc := range plan.ResourceChanges {
		action, skip, err := parseActions(rc.Change.Actions)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}

		lvl := evaluateLevel(rc.Type, action, rules)
		var attrs []string
		if action == ActionCreate || action == ActionDelete {
			attrs = []string{}
		} else {
			attrs = extractAttributes(rc.Change)
		}

		change := Change{
			Address:    rc.Address,
			Type:       rc.Type,
			Action:     action,
			Level:      lvl,
			Attributes: attrs,
		}
		summary.Changes = append(summary.Changes, change)
		summary.Counts[action]++
		if lvl > summary.MaxLevel {
			summary.MaxLevel = lvl
		}
	}

	sort.SliceStable(summary.Changes, func(i, j int) bool {
		if summary.Changes[i].Level != summary.Changes[j].Level {
			return summary.Changes[i].Level > summary.Changes[j].Level
		}
		return summary.Changes[i].Address < summary.Changes[j].Address
	})

	return summary, nil
}

func parseActions(actions []string) (Action, bool, error) {
	if len(actions) == 0 {
		return "", false, fmt.Errorf("%w: actions list is empty", ErrInvalidPlan)
	}
	if len(actions) == 1 {
		switch actions[0] {
		case "create":
			return ActionCreate, false, nil
		case "update":
			return ActionUpdate, false, nil
		case "delete":
			return ActionDelete, false, nil
		case "read", "no-op":
			return "", true, nil
		default:
			return "", false, fmt.Errorf("%w: unsupported action %q", ErrInvalidPlan, actions[0])
		}
	}
	if len(actions) == 2 {
		if (actions[0] == "delete" && actions[1] == "create") ||
			(actions[0] == "create" && actions[1] == "delete") {
			return ActionReplace, false, nil
		}
	}
	return "", false, fmt.Errorf("%w: unsupported action sequence %v", ErrInvalidPlan, actions)
}

func baseLevel(action Action) Level {
	switch action {
	case ActionCreate:
		return LevelLow
	case ActionUpdate:
		return LevelMedium
	case ActionDelete, ActionReplace:
		return LevelHigh
	default:
		return LevelNone
	}
}

func evaluateLevel(resType string, action Action, rules []Rule) Level {
	lvl := baseLevel(action)
	for _, r := range rules {
		matched, err := path.Match(r.TypePattern, resType)
		if err != nil || !matched {
			continue
		}
		if len(r.Actions) > 0 {
			actionMatched := false
			for _, a := range r.Actions {
				if a == action {
					actionMatched = true
					break
				}
			}
			if !actionMatched {
				continue
			}
		}
		if r.Level > lvl {
			lvl = r.Level
		}
	}
	return lvl
}

func extractAttributes(c rawChange) []string {
	beforeMap, _ := c.Before.(map[string]any)
	afterMap, _ := c.After.(map[string]any)
	unknownMap, _ := c.AfterUnknown.(map[string]any)

	keySet := make(map[string]struct{})
	for k := range beforeMap {
		keySet[k] = struct{}{}
	}
	for k := range afterMap {
		keySet[k] = struct{}{}
	}
	for k := range unknownMap {
		keySet[k] = struct{}{}
	}

	if len(keySet) == 0 {
		return []string{}
	}

	sortedKeys := make([]string, 0, len(keySet))
	for k := range keySet {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	attrs := make([]string, 0, len(sortedKeys))
	for _, k := range sortedKeys {
		unknown := isKeyMarked(k, c.AfterUnknown)
		changed := false
		if unknown {
			changed = true
		} else {
			var bVal, aVal any
			var bOk, aOk bool
			if beforeMap != nil {
				bVal, bOk = beforeMap[k]
			}
			if afterMap != nil {
				aVal, aOk = afterMap[k]
			}
			if bOk != aOk {
				changed = true
			} else if bOk && aOk {
				if !reflect.DeepEqual(bVal, aVal) {
					changed = true
				}
			}
		}

		if !changed {
			continue
		}

		attrStr := k
		if unknown {
			attrStr += " (known after apply)"
		}
		if isKeyMarked(k, c.BeforeSensitive) || isKeyMarked(k, c.AfterSensitive) {
			attrStr += " (sensitive)"
		}
		attrs = append(attrs, attrStr)
	}

	return attrs
}

func isKeyMarked(k string, data any) bool {
	if data == nil {
		return false
	}
	if b, ok := data.(bool); ok {
		return b
	}
	if m, ok := data.(map[string]any); ok {
		return isMarked(m[k])
	}
	return false
}

func isMarked(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case map[string]any:
		for _, child := range val {
			if isMarked(child) {
				return true
			}
		}
	case []any:
		for _, child := range val {
			if isMarked(child) {
				return true
			}
		}
	}
	return false
}

// RenderMarkdown formats the plan summary as GitHub-flavored Markdown.
func RenderMarkdown(s *Summary) string {
	if s == nil || len(s.Changes) == 0 {
		maxLvl := LevelNone
		if s != nil {
			maxLvl = s.MaxLevel
		}
		cCreate, cUpdate, cReplace, cDelete := 0, 0, 0, 0
		if s != nil && s.Counts != nil {
			cCreate = s.Counts[ActionCreate]
			cUpdate = s.Counts[ActionUpdate]
			cReplace = s.Counts[ActionReplace]
			cDelete = s.Counts[ActionDelete]
		}
		return fmt.Sprintf("Plan: %d to create, %d to update, %d to replace, %d to delete. Highest risk: %s.\n",
			cCreate, cUpdate, cReplace, cDelete, maxLvl.String())
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Plan: %d to create, %d to update, %d to replace, %d to delete. Highest risk: %s.\n\n",
		s.Counts[ActionCreate],
		s.Counts[ActionUpdate],
		s.Counts[ActionReplace],
		s.Counts[ActionDelete],
		s.MaxLevel.String(),
	)
	b.WriteString("| Risk | Action | Address | Changed |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, c := range s.Changes {
		addr := strings.ReplaceAll(c.Address, "|", `\|`)
		var changedStr string
		if len(c.Attributes) == 0 {
			changedStr = "-"
		} else {
			escapedAttrs := make([]string, len(c.Attributes))
			for i, a := range c.Attributes {
				escapedAttrs[i] = strings.ReplaceAll(a, "|", `\|`)
			}
			if len(escapedAttrs) <= 20 {
				changedStr = strings.Join(escapedAttrs, ", ")
			} else {
				first20 := append(escapedAttrs[:20], fmt.Sprintf("+%d more", len(escapedAttrs)-20))
				changedStr = strings.Join(first20, ", ")
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.Level.String(), c.Action, addr, changedStr)
	}
	return b.String()
}

// Run executes the CLI command with the given arguments and I/O streams.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tf-plan-summary", flag.ContinueOnError)
	fs.SetOutput(stderr)

	formatFlag := fs.String("format", "markdown", "output format: markdown or json")
	failOnFlag := fs.String("fail-on", "none", "fail with exit code 1 at or above risk level: none, low, medium, high, critical")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *formatFlag != "markdown" && *formatFlag != "json" {
		fmt.Fprintf(stderr, "error: invalid format %q, must be markdown or json\n", *formatFlag)
		return 2
	}

	failLevel, err := ParseLevel(*failOnFlag)
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid --fail-on level %q: %v\n", *failOnFlag, err)
		return 2
	}

	posArgs := fs.Args()
	if len(posArgs) == 0 {
		fmt.Fprintf(stderr, "error: missing plan file argument\n")
		return 2
	}

	filePath := posArgs[0]
	var r io.Reader
	if filePath == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(filePath)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		defer f.Close()
		r = f
	}

	summary, err := Parse(r, Options{})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	if *formatFlag == "json" {
		data, err := json.Marshal(summary)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		stdout.Write(append(data, '\n'))
	} else {
		out := RenderMarkdown(summary)
		io.WriteString(stdout, out)
	}

	if failLevel > LevelNone && summary.MaxLevel >= failLevel {
		return 1
	}

	return 0
}
