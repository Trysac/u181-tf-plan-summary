package tfplansummary_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tfplansummary "github.com/Trysac/u181-tf-plan-summary"
)

func TestAcceptance1_ActionsAndRiskLevels(t *testing.T) {
	// Acceptance 1: For a plan with aws_db_instance.main actions ["delete","create"],
	// aws_instance.web ["update"], random_id.x ["create"] and data.aws_ami.x ["read"],
	// Summary.Changes addresses are ["aws_db_instance.main", "aws_instance.web", "random_id.x"]
	// with actions "replace", "update", "create" and levels "critical", "medium", "low";
	// reads and no-ops are excluded and MaxLevel is LevelCritical.
	planJSON := `{
		"format_version": "1.1",
		"resource_changes": [
			{
				"address": "random_id.x",
				"type": "random_id",
				"change": {"actions": ["create"]}
			},
			{
				"address": "aws_instance.web",
				"type": "aws_instance",
				"change": {"actions": ["update"]}
			},
			{
				"address": "aws_db_instance.main",
				"type": "aws_db_instance",
				"change": {"actions": ["delete", "create"]}
			},
			{
				"address": "data.aws_ami.x",
				"type": "aws_ami",
				"change": {"actions": ["read"]}
			}
		]
	}`

	summary, err := tfplansummary.Parse(strings.NewReader(planJSON), tfplansummary.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(summary.Changes) != 3 {
		t.Fatalf("expected 3 changes, got %d", len(summary.Changes))
	}

	wantAddresses := []string{"aws_db_instance.main", "aws_instance.web", "random_id.x"}
	wantActions := []tfplansummary.Action{tfplansummary.ActionReplace, tfplansummary.ActionUpdate, tfplansummary.ActionCreate}
	wantLevels := []tfplansummary.Level{tfplansummary.LevelCritical, tfplansummary.LevelMedium, tfplansummary.LevelLow}

	for i, c := range summary.Changes {
		if c.Address != wantAddresses[i] {
			t.Errorf("change[%d].Address = %q, want %q", i, c.Address, wantAddresses[i])
		}
		if c.Action != wantActions[i] {
			t.Errorf("change[%d].Action = %q, want %q", i, c.Action, wantActions[i])
		}
		if c.Level != wantLevels[i] {
			t.Errorf("change[%d].Level = %v, want %v", i, c.Level, wantLevels[i])
		}
	}

	if summary.MaxLevel != tfplansummary.LevelCritical {
		t.Errorf("summary.MaxLevel = %v, want %v", summary.MaxLevel, tfplansummary.LevelCritical)
	}
}

func TestAcceptance2_AttributesDiffAndInvalidAction(t *testing.T) {
	// Acceptance 2: An update with before {"ami":"a","tags":{"a":"1"}},
	// after {"ami":"a","tags":{"a":"2"}} and after_unknown {"id":true}
	// has Attributes ["id (known after apply)", "tags"];
	// ["create","delete"] is also "replace";
	// ["delete","update"] returns an error matching ErrInvalidPlan with errors.Is.
	planJSON := `{
		"format_version": "1.0",
		"resource_changes": [
			{
				"address": "aws_instance.test",
				"type": "aws_instance",
				"change": {
					"actions": ["update"],
					"before": {"ami": "a", "tags": {"a": "1"}},
					"after": {"ami": "a", "tags": {"a": "2"}},
					"after_unknown": {"id": true}
				}
			}
		]
	}`

	summary, err := tfplansummary.Parse(strings.NewReader(planJSON), tfplansummary.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summary.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(summary.Changes))
	}
	gotAttrs := summary.Changes[0].Attributes
	wantAttrs := []string{"id (known after apply)", "tags"}
	if len(gotAttrs) != len(wantAttrs) {
		t.Fatalf("Attributes length = %d, want %d (%v)", len(gotAttrs), len(wantAttrs), gotAttrs)
	}
	for i := range wantAttrs {
		if gotAttrs[i] != wantAttrs[i] {
			t.Errorf("attribute[%d] = %q, want %q", i, gotAttrs[i], wantAttrs[i])
		}
	}

	// Check ["create", "delete"] is also replace
	replaceJSON := `{
		"format_version": "1.0",
		"resource_changes": [
			{
				"address": "aws_instance.test",
				"type": "aws_instance",
				"change": {
					"actions": ["create", "delete"]
				}
			}
		]
	}`
	replaceSummary, err := tfplansummary.Parse(strings.NewReader(replaceJSON), tfplansummary.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if replaceSummary.Changes[0].Action != tfplansummary.ActionReplace {
		t.Errorf("action = %q, want %q", replaceSummary.Changes[0].Action, tfplansummary.ActionReplace)
	}

	// Check ["delete", "update"] returns an error matching ErrInvalidPlan
	invalidJSON := `{
		"format_version": "1.0",
		"resource_changes": [
			{
				"address": "aws_instance.test",
				"type": "aws_instance",
				"change": {
					"actions": ["delete", "update"]
				}
			}
		]
	}`
	_, err = tfplansummary.Parse(strings.NewReader(invalidJSON), tfplansummary.Options{})
	if !errors.Is(err, tfplansummary.ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got %v", err)
	}
}

func TestAcceptance3_SensitiveAttributesNeverExposeValues(t *testing.T) {
	// Acceptance 3: With before_sensitive {"password":true} and a changed password
	// value "plain-sentinel", Attributes contains "password (sensitive)" and neither
	// RenderMarkdown nor the JSON output contains "plain-sentinel";
	// no output ever contains a before or after value.
	sentinel := "plain-sentinel"
	planJSON := fmt.Sprintf(`{
		"format_version": "1.0",
		"resource_changes": [
			{
				"address": "vault_generic_secret.test",
				"type": "vault_generic_secret",
				"change": {
					"actions": ["update"],
					"before": {"password": %q},
					"after": {"password": "updated-secret"},
					"before_sensitive": {"password": true}
				}
			}
		]
	}`, sentinel)

	summary, err := tfplansummary.Parse(strings.NewReader(planJSON), tfplansummary.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(summary.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(summary.Changes))
	}

	attrs := summary.Changes[0].Attributes
	found := false
	for _, a := range attrs {
		if a == "password (sensitive)" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Attributes to contain %q, got %v", "password (sensitive)", attrs)
	}

	md := tfplansummary.RenderMarkdown(summary)
	if strings.Contains(md, sentinel) {
		t.Fatalf("Markdown output leaked secret value %q", sentinel)
	}

	jsonBytes, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	if strings.Contains(string(jsonBytes), sentinel) {
		t.Fatalf("JSON output leaked secret value %q", sentinel)
	}
	if strings.Contains(string(jsonBytes), "updated-secret") {
		t.Fatalf("JSON output leaked after value")
	}
}

func TestAcceptance4_RulesOverrideAndBaseLevelFloor(t *testing.T) {
	// Acceptance 4: Rule{TypePattern: "aws_instance", Actions: []Action{ActionUpdate}, Level: LevelHigh}
	// raises aws_instance.web to "high"; DefaultRules makes "aws_iam_role" with action "create" "high"
	// through pattern "*iam*", and a lower rule level never lowers the base action level.
	planJSON := `{
		"format_version": "1.0",
		"resource_changes": [
			{
				"address": "aws_instance.web",
				"type": "aws_instance",
				"change": {"actions": ["update"]}
			},
			{
				"address": "aws_iam_role.admin",
				"type": "aws_iam_role",
				"change": {"actions": ["create"]}
			},
			{
				"address": "aws_instance.old",
				"type": "aws_instance",
				"change": {"actions": ["delete"]}
			}
		]
	}`

	customRules := []tfplansummary.Rule{
		{
			TypePattern: "aws_instance",
			Actions:     []tfplansummary.Action{tfplansummary.ActionUpdate},
			Level:       tfplansummary.LevelHigh,
		},
		{
			TypePattern: "*iam*",
			Actions:     nil,
			Level:       tfplansummary.LevelHigh,
		},
		{
			// Lower rule: should not lower delete from base High to Low
			TypePattern: "aws_instance",
			Actions:     []tfplansummary.Action{tfplansummary.ActionDelete},
			Level:       tfplansummary.LevelLow,
		},
	}

	summary, err := tfplansummary.Parse(strings.NewReader(planJSON), tfplansummary.Options{Rules: customRules})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range summary.Changes {
		switch c.Address {
		case "aws_instance.web":
			if c.Level != tfplansummary.LevelHigh {
				t.Errorf("aws_instance.web level = %v, want LevelHigh", c.Level)
			}
		case "aws_iam_role.admin":
			if c.Level != tfplansummary.LevelHigh {
				t.Errorf("aws_iam_role.admin level = %v, want LevelHigh", c.Level)
			}
		case "aws_instance.old":
			if c.Level != tfplansummary.LevelHigh {
				t.Errorf("aws_instance.old delete level was lowered to %v, want base LevelHigh", c.Level)
			}
		}
	}
}

func TestAcceptance5_ErrorsAndLimits(t *testing.T) {
	// Acceptance 5: format_version "2.0" returns ErrUnsupportedFormat,
	// a body of Options.MaxBytes+1 bytes returns ErrTooLarge while MaxBytes is accepted,
	// and "{" returns ErrInvalidPlan, each checked with errors.Is.

	// format_version 2.0
	v2Plan := `{"format_version": "2.0", "resource_changes": []}`
	_, err := tfplansummary.Parse(strings.NewReader(v2Plan), tfplansummary.Options{})
	if !errors.Is(err, tfplansummary.ErrUnsupportedFormat) {
		t.Errorf("format_version 2.0 error = %v, want ErrUnsupportedFormat", err)
	}

	// MaxBytes check
	validBody := `{"format_version":"1.0","resource_changes":[]}`
	maxLen := int64(len(validBody))

	// MaxBytes accepted
	_, err = tfplansummary.Parse(strings.NewReader(validBody), tfplansummary.Options{MaxBytes: maxLen})
	if err != nil {
		t.Errorf("MaxBytes exact limit failed: %v", err)
	}

	// MaxBytes+1 returns ErrTooLarge
	_, err = tfplansummary.Parse(strings.NewReader(validBody+" "), tfplansummary.Options{MaxBytes: maxLen})
	if !errors.Is(err, tfplansummary.ErrTooLarge) {
		t.Errorf("MaxBytes+1 error = %v, want ErrTooLarge", err)
	}

	// "{" returns ErrInvalidPlan
	_, err = tfplansummary.Parse(strings.NewReader("{"), tfplansummary.Options{})
	if !errors.Is(err, tfplansummary.ErrInvalidPlan) {
		t.Errorf("malformed JSON error = %v, want ErrInvalidPlan", err)
	}
}

func TestAcceptance6_RenderMarkdownRowsAndEscaping(t *testing.T) {
	// Acceptance 6: RenderMarkdown starts with "Plan: 1 to create, 1 to update, 1 to replace, 0 to delete. Highest risk: critical.\n"
	// and contains the row "| critical | replace | aws_db_instance.main | - |";
	// an address aws_x.y["a|b"] is written as aws_x.y["a\|b"].
	s := &tfplansummary.Summary{
		Counts: map[tfplansummary.Action]int{
			tfplansummary.ActionCreate:  1,
			tfplansummary.ActionUpdate:  1,
			tfplansummary.ActionReplace: 1,
			tfplansummary.ActionDelete:  0,
		},
		MaxLevel: tfplansummary.LevelCritical,
		Changes: []tfplansummary.Change{
			{
				Address:    "aws_db_instance.main",
				Type:       "aws_db_instance",
				Action:     tfplansummary.ActionReplace,
				Level:      tfplansummary.LevelCritical,
				Attributes: []string{},
			},
			{
				Address:    `aws_x.y["a|b"]`,
				Type:       "aws_x",
				Action:     tfplansummary.ActionUpdate,
				Level:      tfplansummary.LevelMedium,
				Attributes: []string{"attr1"},
			},
			{
				Address:    "random_id.x",
				Type:       "random_id",
				Action:     tfplansummary.ActionCreate,
				Level:      tfplansummary.LevelLow,
				Attributes: []string{},
			},
		},
	}

	md := tfplansummary.RenderMarkdown(s)
	wantPrefix := "Plan: 1 to create, 1 to update, 1 to replace, 0 to delete. Highest risk: critical.\n"
	if !strings.HasPrefix(md, wantPrefix) {
		t.Fatalf("RenderMarkdown prefix = %q, want %q", md[:len(wantPrefix)], wantPrefix)
	}

	wantRow1 := "| critical | replace | aws_db_instance.main | - |"
	if !strings.Contains(md, wantRow1) {
		t.Errorf("RenderMarkdown missing row: %q\nGot:\n%s", wantRow1, md)
	}

	wantEscapedAddr := `aws_x.y["a\|b"]`
	if !strings.Contains(md, wantEscapedAddr) {
		t.Errorf("RenderMarkdown missing escaped address: %q\nGot:\n%s", wantEscapedAddr, md)
	}
}

func TestAcceptance7_EmptyPlanAndOver20Attributes(t *testing.T) {
	// Acceptance 7: An empty resource_changes array renders exactly
	// "Plan: 0 to create, 0 to update, 0 to replace, 0 to delete. Highest risk: none.\n";
	// more than 20 changed attributes list 20 sorted names followed by '+N more'.
	emptyPlan := `{"format_version": "1.0", "resource_changes": []}`
	summary, err := tfplansummary.Parse(strings.NewReader(emptyPlan), tfplansummary.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotMD := tfplansummary.RenderMarkdown(summary)
	wantMD := "Plan: 0 to create, 0 to update, 0 to replace, 0 to delete. Highest risk: none.\n"
	if gotMD != wantMD {
		t.Fatalf("RenderMarkdown on empty plan = %q, want %q", gotMD, wantMD)
	}

	// Plan with 25 changed attributes
	beforeMap := make(map[string]any)
	afterMap := make(map[string]any)
	for i := 1; i <= 25; i++ {
		k := fmt.Sprintf("attr_%02d", i)
		beforeMap[k] = i
		afterMap[k] = i + 100
	}

	planObj := map[string]any{
		"format_version": "1.0",
		"resource_changes": []map[string]any{
			{
				"address": "aws_instance.test",
				"type":    "aws_instance",
				"change": map[string]any{
					"actions": []string{"update"},
					"before":  beforeMap,
					"after":   afterMap,
				},
			},
		},
	}
	data, err := json.Marshal(planObj)
	if err != nil {
		t.Fatalf("failed to marshal test plan: %v", err)
	}

	s25, err := tfplansummary.Parse(bytes.NewReader(data), tfplansummary.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s25.Changes[0].Attributes) != 25 {
		t.Fatalf("expected 25 attributes, got %d", len(s25.Changes[0].Attributes))
	}

	md25 := tfplansummary.RenderMarkdown(s25)
	if !strings.Contains(md25, "+5 more") {
		t.Errorf("RenderMarkdown for 25 attributes missing '+5 more':\n%s", md25)
	}
	// Ensure attr_01 through attr_20 are in the row
	if !strings.Contains(md25, "attr_20, +5 more") {
		t.Errorf("RenderMarkdown missing 'attr_20, +5 more':\n%s", md25)
	}
	if strings.Contains(md25, "attr_21") {
		t.Errorf("RenderMarkdown should not display attr_21 directly:\n%s", md25)
	}
}

func TestAcceptance8_RunCLI(t *testing.T) {
	// Acceptance 8: Run([]string{"--fail-on", "high", "plan.json"}, stdout, stderr)
	// returns 1 when MaxLevel >= LevelHigh and 0 otherwise;
	// "--format", "json" prints {"changes":[...],"counts":{...},"max_level":"critical"};
	// a missing file returns 2 with stderr "error: open plan.json" prefix.

	tmpDir := t.TempDir()
	highPlanPath := filepath.Join(tmpDir, "high_plan.json")
	lowPlanPath := filepath.Join(tmpDir, "low_plan.json")

	highPlanJSON := `{
		"format_version": "1.0",
		"resource_changes": [
			{
				"address": "aws_s3_bucket.b",
				"type": "aws_s3_bucket",
				"change": {"actions": ["delete"]}
			}
		]
	}`
	lowPlanJSON := `{
		"format_version": "1.0",
		"resource_changes": [
			{
				"address": "random_id.x",
				"type": "random_id",
				"change": {"actions": ["create"]}
			}
		]
	}`

	if err := os.WriteFile(highPlanPath, []byte(highPlanJSON), 0600); err != nil {
		t.Fatalf("failed to write high_plan.json: %v", err)
	}
	if err := os.WriteFile(lowPlanPath, []byte(lowPlanJSON), 0600); err != nil {
		t.Fatalf("failed to write low_plan.json: %v", err)
	}

	// Test 1: returns 1 when MaxLevel >= LevelHigh (high_plan is critical due to aws_s3_bucket delete)
	var stdout, stderr bytes.Buffer
	code := tfplansummary.Run([]string{"--fail-on", "high", highPlanPath}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("Run() with high plan = %d, want 1. stderr: %s", code, stderr.String())
	}

	// Test 2: returns 0 when MaxLevel < LevelHigh
	stdout.Reset()
	stderr.Reset()
	code = tfplansummary.Run([]string{"--fail-on", "high", lowPlanPath}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("Run() with low plan = %d, want 0. stderr: %s", code, stderr.String())
	}

	// Test 3: "--format", "json" prints {"changes":[...],"counts":{...},"max_level":"critical"}
	stdout.Reset()
	stderr.Reset()
	code = tfplansummary.Run([]string{"--format", "json", highPlanPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run() with --format json = %d, want 0. stderr: %s", code, stderr.String())
	}
	outStr := stdout.String()
	if !strings.Contains(outStr, `"changes":[`) || !strings.Contains(outStr, `"counts":{`) || !strings.Contains(outStr, `"max_level":"critical"`) {
		t.Errorf("JSON output missing expected keys/values. Got:\n%s", outStr)
	}

	// Test 4: a missing file returns 2 with stderr "error: open plan.json" prefix
	stdout.Reset()
	stderr.Reset()
	code = tfplansummary.Run([]string{"plan.json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("Run() with missing file = %d, want 2", code)
	}
	if !strings.HasPrefix(stderr.String(), "error: open plan.json") {
		t.Errorf("stderr = %q, want prefix 'error: open plan.json'", stderr.String())
	}
}

func TestEdgeCases_ModulesAndIndexes(t *testing.T) {
	planJSON := `{
		"format_version": "1.2",
		"resource_changes": [
			{
				"address": "module.app.aws_s3_bucket.bucket[0]",
				"type": "aws_s3_bucket",
				"change": {"actions": ["delete"]}
			},
			{
				"address": "aws_security_group_rule.ingress[\"http\"]",
				"type": "aws_security_group_rule",
				"change": {"actions": ["no-op"]}
			}
		]
	}`

	summary, err := tfplansummary.Parse(strings.NewReader(planJSON), tfplansummary.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summary.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(summary.Changes))
	}
	c := summary.Changes[0]
	if c.Level != tfplansummary.LevelCritical {
		t.Errorf("module s3 delete level = %v, want LevelCritical", c.Level)
	}
}

func TestLevel_StringAndJSON(t *testing.T) {
	levels := []struct {
		l    tfplansummary.Level
		want string
	}{
		{tfplansummary.LevelNone, "none"},
		{tfplansummary.LevelLow, "low"},
		{tfplansummary.LevelMedium, "medium"},
		{tfplansummary.LevelHigh, "high"},
		{tfplansummary.LevelCritical, "critical"},
		{tfplansummary.Level(99), "level(99)"},
	}
	for _, tt := range levels {
		if got := tt.l.String(); got != tt.want {
			t.Errorf("Level(%d).String() = %q, want %q", int(tt.l), got, tt.want)
		}
		b, err := json.Marshal(tt.l)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if string(b) != fmt.Sprintf("%q", tt.want) {
			t.Errorf("MarshalJSON = %s, want %q", string(b), tt.want)
		}
		if tt.l <= tfplansummary.LevelCritical {
			var unmarshaled tfplansummary.Level
			if err := json.Unmarshal(b, &unmarshaled); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if unmarshaled != tt.l {
				t.Errorf("Unmarshal = %v, want %v", unmarshaled, tt.l)
			}
		}
	}

	var invalidLvl tfplansummary.Level
	if err := json.Unmarshal([]byte(`"unknown"`), &invalidLvl); err == nil {
		t.Errorf("expected error for unmarshaling unknown level string")
	}
	if err := json.Unmarshal([]byte(`123`), &invalidLvl); err == nil {
		t.Errorf("expected error for unmarshaling number into level")
	}
}

func TestRunCLI_Errors(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// Unknown flag
	code := tfplansummary.Run([]string{"--unknown"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("expected exit 2 for unknown flag, got %d", code)
	}

	// Invalid format
	stderr.Reset()
	code = tfplansummary.Run([]string{"--format", "yaml", "file.json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("expected exit 2 for invalid format, got %d", code)
	}

	// Invalid fail-on
	stderr.Reset()
	code = tfplansummary.Run([]string{"--fail-on", "danger", "file.json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("expected exit 2 for invalid fail-on, got %d", code)
	}

	// Missing argument
	stderr.Reset()
	code = tfplansummary.Run([]string{}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("expected exit 2 for missing argument, got %d", code)
	}

	// Invalid JSON file
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad.json")
	os.WriteFile(badFile, []byte("{"), 0600)
	stderr.Reset()
	code = tfplansummary.Run([]string{badFile}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("expected exit 2 for invalid JSON, got %d", code)
	}
}

func TestParse_MissingFormatVersion(t *testing.T) {
	planJSON := `{"resource_changes": []}`
	_, err := tfplansummary.Parse(strings.NewReader(planJSON), tfplansummary.Options{})
	if !errors.Is(err, tfplansummary.ErrInvalidPlan) {
		t.Errorf("missing format_version error = %v, want ErrInvalidPlan", err)
	}
}

func TestParse_ActionsErrors(t *testing.T) {
	tests := []struct {
		name    string
		actions []string
	}{
		{"empty", []string{}},
		{"invalid-single", []string{"destroy"}},
		{"invalid-triple", []string{"create", "update", "delete"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := map[string]any{
				"format_version": "1.0",
				"resource_changes": []map[string]any{
					{
						"address": "test.addr",
						"type":    "test",
						"change": map[string]any{
							"actions": tt.actions,
						},
					},
				},
			}
			b, _ := json.Marshal(plan)
			_, err := tfplansummary.Parse(bytes.NewReader(b), tfplansummary.Options{})
			if !errors.Is(err, tfplansummary.ErrInvalidPlan) {
				t.Errorf("actions %v error = %v, want ErrInvalidPlan", tt.actions, err)
			}
		})
	}
}

func TestProtectedResources_CriticalRules(t *testing.T) {
	protected := []string{
		"aws_db_instance",
		"aws_rds_cluster",
		"aws_s3_bucket",
		"aws_dynamodb_table",
		"google_sql_database_instance",
		"google_storage_bucket",
		"azurerm_mssql_database",
		"azurerm_storage_account",
	}

	for _, resType := range protected {
		plan := map[string]any{
			"format_version": "1.0",
			"resource_changes": []map[string]any{
				{
					"address": resType + ".item",
					"type":    resType,
					"change": map[string]any{
						"actions": []string{"delete"},
					},
				},
			},
		}
		b, _ := json.Marshal(plan)
		summary, err := tfplansummary.Parse(bytes.NewReader(b), tfplansummary.Options{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary.Changes[0].Level != tfplansummary.LevelCritical {
			t.Errorf("%s delete level = %v, want LevelCritical", resType, summary.Changes[0].Level)
		}
	}
}

func TestRenderMarkdown_NilSummary(t *testing.T) {
	got := tfplansummary.RenderMarkdown(nil)
	want := "Plan: 0 to create, 0 to update, 0 to replace, 0 to delete. Highest risk: none.\n"
	if got != want {
		t.Errorf("RenderMarkdown(nil) = %q, want %q", got, want)
	}
}
