package rotation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opencode2api/internal/config"
)

func newManager(t *testing.T, path string) *Manager {
	t.Helper()
	cfg := config.RotationConfig{}
	cfg.Defaults()
	m, err := New(path, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

func sotaAll() map[string]bool {
	available := map[string]bool{}
	for _, model := range config.SOTAModels {
		available[model] = true
	}
	return available
}

func sweetModels() map[string]bool {
	return map[string]bool{"glm-5.3-flash": true, "deepseek-v3": true, "qwen-plus": true}
}

func all() map[string]bool {
	merged := sotaAll()
	for model := range sweetModels() {
		merged[model] = true
	}
	return merged
}

func current(t *testing.T, m *Manager, id string) (string, uint64) {
	t.Helper()
	snap := m.Snapshot()
	for _, g := range snap.Groups {
		if g.ID == id {
			return g.Current, g.Generation
		}
	}
	t.Fatalf("group %q missing from snapshot", id)
	return "", 0
}

func TestConfigureAppliesDefaults(t *testing.T) {
	m := newManager(t, "")
	snap := m.Snapshot()
	if len(snap.Groups) != 2 {
		t.Fatalf("want 2 groups, got %d", len(snap.Groups))
	}
	if snap.Groups[0].ID != "sota" || snap.Groups[1].ID != "sweet" {
		t.Fatalf("unexpected group ids: %s, %s", snap.Groups[0].ID, snap.Groups[1].ID)
	}
	if snap.Groups[0].Alias != "sota" || snap.Groups[1].Alias != "sweet" {
		t.Fatalf("aliases not defaulted: %q, %q", snap.Groups[0].Alias, snap.Groups[1].Alias)
	}
	if snap.FirstOutputSeconds != 30 {
		t.Fatalf("FirstOutputSeconds = %d, want 30", snap.FirstOutputSeconds)
	}
	// The SOTA group starts on the first configured model.
	if got := snap.Groups[0].Current; got != config.SOTAModels[0] {
		t.Fatalf("sota current = %q", got)
	}
}

func TestGroupIDResolvesAlias(t *testing.T) {
	m := newManager(t, "")
	if got := m.GroupID("sota"); got != "sota" {
		t.Fatalf("GroupID(sota) = %q", got)
	}
	if got := m.GroupID("sweet"); got != "sweet" {
		t.Fatalf("GroupID(sweet) = %q", got)
	}
	if got := m.GroupID("nope"); got != "" {
		t.Fatalf("GroupID(unknown) = %q, want empty", got)
	}
}

func TestConfigureKeepsCurrentWhenStillInOrder(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())
	m.Manual("sota", config.SOTAModels[3])

	cfg := config.RotationConfig{}
	cfg.Defaults()
	// Re-applying the same order must not reset the cursor.
	m.Configure(cfg)
	if got, _ := current(t, m, "sota"); got != config.SOTAModels[3] {
		t.Fatalf("re-configure moved the cursor to %q", got)
	}
}

func TestConfigureAppendsUnknownModels(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())
	if err := m.Manual("sota", config.SOTAModels[0]); err != nil {
		t.Fatalf("Manual: %v", err)
	}

	// A new SOTA model appears upstream and is appended, not inserted.
	cfg := config.RotationConfig{
		SOTA: config.RotationGroup{
			Alias: "sota",
			Order: append(append([]string{}, config.SOTAModels...), "new-sota-model"),
		},
		Sweet:              config.RotationGroup{Alias: "sweet"},
		FirstOutputSeconds: 30,
	}
	m.Configure(cfg)

	var order []string
	for _, g := range m.Snapshot().Groups {
		if g.ID == "sota" {
			order = g.Order
		}
	}
	found := false
	for _, model := range order {
		if model == "new-sota-model" {
			found = true
		}
	}
	if !found {
		t.Fatalf("new model not appended: %v", order)
	}
}

func TestSyncPartitionsAvailabilityByGroup(t *testing.T) {
	m := newManager(t, "")
	m.Sync(all())

	snap := m.Snapshot()
	for _, g := range snap.Groups {
		for model := range g.Available {
			if config.IsSOTA(model) != (g.ID == "sota") {
				t.Errorf("%s group claims SOTA-ineligible model %q", g.ID, model)
			}
		}
	}
}

// glm-5.3-flash leads the sweet group so a fresh install starts on it.
func TestSyncPromotesGlmFlashToFrontOfSweet(t *testing.T) {
	m := newManager(t, "")
	m.Sync(all())

	var order []string
	for _, g := range m.Snapshot().Groups {
		if g.ID == "sweet" {
			order = g.Order
		}
	}
	if len(order) == 0 {
		t.Fatal("sweet order should not be empty after sync")
	}
	if order[0] != "glm-5.3-flash" {
		t.Fatalf("sweet order[0] = %q, want glm-5.3-flash", order[0])
	}
	// Remaining models follow in sorted order.
	if len(order) != 3 {
		t.Fatalf("sweet order = %v, want 3 entries", order)
	}
}

// Newly discovered models join the group they belong to. Membership is decided
// by config.IsSOTA, so a late addition can only appear in the sweet group.
func TestSyncAppendsLateModels(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	before, _ := current(t, m, "sota")
	extra := sotaAll()
	extra["brand-new-sweet-model"] = true
	m.Sync(extra)

	if after, _ := current(t, m, "sota"); after != before {
		t.Fatalf("sync moved the sota cursor from %q to %q", before, after)
	}
	var sweet []string
	for _, g := range m.Snapshot().Groups {
		if g.ID == "sweet" {
			sweet = g.Order
		}
	}
	found := false
	for _, model := range sweet {
		if model == "brand-new-sweet-model" {
			found = true
		}
	}
	if !found {
		t.Fatalf("late model should be appended to the sweet order: %v", sweet)
	}
}

func TestSelectFailsOnEmptyGroup(t *testing.T) {
	m := newManager(t, "")
	if _, ok := m.Select("sweet", nil); ok {
		t.Fatal("sweet group has no order yet")
	}
	if _, ok := m.Select("missing", nil); ok {
		t.Fatal("unknown group should not select")
	}
}

func TestSelectPrefersCurrentModel(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	tok, ok := m.Select("sota", nil)
	if !ok {
		t.Fatal("expected a selection")
	}
	if tok.Model != config.SOTAModels[0] {
		t.Fatalf("Select = %q, want the current model %q", tok.Model, config.SOTAModels[0])
	}
}

func TestSelectSkipsVisitedWithoutMovingGlobalCursor(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	before, _ := current(t, m, "sota")
	tok, ok := m.Select("sota", map[string]bool{before: true})
	if !ok {
		t.Fatal("expected a selection")
	}
	if tok.Model == before {
		t.Fatal("visited model should be skipped")
	}
	// A request-local skip must not disturb the shared selection.
	if after, _ := current(t, m, "sota"); after != before {
		t.Fatalf("global cursor moved from %q to %q on a local skip", before, after)
	}
}

func TestSelectMovesGlobalCursorWhenCurrentUnusable(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	before, generation := current(t, m, "sota")
	// A local skip alone does not move the cursor...
	m.Select("sota", map[string]bool{before: true})
	if after, _ := current(t, m, "sota"); after != before {
		t.Fatal("cursor should not move while the current model is healthy")
	}

	// ...but a failure of the current model, carrying its live generation,
	// pauses it and advances the shared cursor.
	m.Fail(Token{Group: "sota", Model: before, Generation: generation}, time.Minute, "upstream error")
	moved, _ := current(t, m, "sota")
	if moved == before {
		t.Fatal("cursor should advance after the current model is paused")
	}
}

func TestFailAdvancesOnlyForCurrentGeneration(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	tok, ok := m.Select("sota", nil)
	if !ok {
		t.Fatal("expected a selection")
	}

	// An operator moves the cursor while the request is still in flight.
	target := config.SOTAModels[4]
	if err := m.Manual("sota", target); err != nil {
		t.Fatalf("Manual: %v", err)
	}

	// The stale failure must not drag the shared cursor back.
	m.Fail(tok, time.Minute, "late failure")

	if got, _ := current(t, m, "sota"); got != target {
		t.Fatalf("stale failure moved the cursor to %q, want %q", got, target)
	}
}

func TestFailPausesOnlyTheFailingModel(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	tok, _ := m.Select("sota", nil)
	m.Fail(tok, time.Minute, "upstream error")

	for _, g := range m.Snapshot().Groups {
		if g.ID != "sota" {
			continue
		}
		if _, paused := g.Paused[tok.Model]; !paused {
			t.Fatalf("model %q should be paused", tok.Model)
		}
		if len(g.Paused) != 1 {
			t.Fatalf("only the failing model should be paused, got %v", g.Paused)
		}
	}
}

func TestManualRejectsUnavailableOrForeignModels(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	if err := m.Manual("sota", "not-in-order"); err == nil {
		t.Fatal("model outside the order should be rejected")
	}
	// A sweet-only model is not part of the sota order.
	if err := m.Manual("sota", "glm-5.3-flash"); err == nil {
		t.Fatal("model from the other group should be rejected")
	}
	if err := m.Manual("missing", config.SOTAModels[0]); err == nil {
		t.Fatal("unknown group should be rejected")
	}
}

func TestManualClearsPauseAndSwitches(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	tok, _ := m.Select("sota", nil)
	m.Fail(tok, time.Hour, "upstream error")
	if _, paused := m.Snapshot().Groups[0].Paused[tok.Model]; !paused {
		t.Fatal("setup: model should be paused")
	}

	if err := m.Manual("sota", tok.Model); err != nil {
		t.Fatalf("Manual on a paused model should work: %v", err)
	}
	g := m.Snapshot().Groups[0]
	if _, paused := g.Paused[tok.Model]; paused {
		t.Fatal("manual selection should clear the pause")
	}
	if g.Current != tok.Model {
		t.Fatalf("current = %q, want %q", g.Current, tok.Model)
	}
	last := g.Switches[len(g.Switches)-1]
	if last.Reason != "管理员手动切换" {
		t.Fatalf("switch reason = %q", last.Reason)
	}
}

func TestSwitchHistoryIsCapped(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	for i := range 40 {
		if err := m.Manual("sota", config.SOTAModels[i%len(config.SOTAModels)]); err != nil {
			t.Fatalf("Manual %d: %v", i, err)
		}
	}
	if got := len(m.Snapshot().Groups[0].Switches); got != 30 {
		t.Fatalf("switch history = %d, want the cap of 30", got)
	}
}

func TestAttemptsAreCappedAtHundred(t *testing.T) {
	m := newManager(t, "")
	for i := range 130 {
		m.Record(Attempt{Request: "r", Alias: "sota", Model: "m", Number: i, Outcome: "success"})
	}
	attempts := m.Snapshot().Attempts
	if len(attempts) != 100 {
		t.Fatalf("attempts = %d, want the cap of 100", len(attempts))
	}
	// The newest entries are the ones kept.
	if attempts[len(attempts)-1].Number != 129 {
		t.Fatalf("newest attempt number = %d, want 129", attempts[len(attempts)-1].Number)
	}
}

func TestRecordStampsServerTime(t *testing.T) {
	m := newManager(t, "")
	before := time.Now().UTC().Add(-time.Second)
	m.Record(Attempt{Request: "r", Outcome: "success"})
	attempts := m.Snapshot().Attempts
	if len(attempts) != 1 {
		t.Fatalf("want 1 attempt, got %d", len(attempts))
	}
	if attempts[0].Time.Before(before) {
		t.Fatalf("attempt time %v was not stamped by the server", attempts[0].Time)
	}
}

func TestStatePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotation.json")

	cfg := config.RotationConfig{}
	cfg.Defaults()
	first, err := New(path, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	first.Sync(sotaAll())
	if err := first.Manual("sota", config.SOTAModels[2]); err != nil {
		t.Fatalf("Manual: %v", err)
	}
	want, _ := current(t, first, "sota")

	second, err := New(path, cfg)
	if err != nil {
		t.Fatalf("restart New: %v", err)
	}
	if got, _ := current(t, second, "sota"); got != want {
		t.Fatalf("cursor %q not restored, got %q", want, got)
	}
}

func TestSnapshotIsADeepCopy(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	snap := m.Snapshot()
	snap.Groups[0].Current = "mutated"

	if got, _ := current(t, m, "sota"); got == "mutated" {
		t.Fatal("Snapshot must not expose live state")
	}
}

func TestNewRejectsUnwritableStatePath(t *testing.T) {
	// A path inside a directory that does not exist cannot be written, and New
	// is documented to refuse to start in that case.
	cfg := config.RotationConfig{}
	cfg.Defaults()
	path := filepath.Join(t.TempDir(), "missing-dir", "rotation.json")
	if _, err := New(path, cfg); err == nil {
		t.Fatal("New should fail when the state file cannot be written")
	}
}

func TestNewRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotation.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.RotationConfig{}
	cfg.Defaults()
	if _, err := New(path, cfg); err == nil {
		t.Fatal("corrupt state should be reported")
	}
}

func TestNewRejectsWrongGroupLayout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rotation.json")
	state := Snapshot{Groups: []*Group{{ID: "sota"}}}
	blob, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.RotationConfig{}
	cfg.Defaults()
	if _, err := New(path, cfg); err == nil {
		t.Fatal("a single-group layout should be rejected")
	}
}

func TestSelectIsSafeUnderConcurrentFailures(t *testing.T) {
	m := newManager(t, "")
	m.Sync(sotaAll())

	// Concurrent readers must not observe a torn or empty cursor.
	done := make(chan struct{})
	for range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 50 {
				m.Select("sota", nil)
				m.Snapshot()
			}
		}()
	}
	for range 8 {
		<-done
	}
	if got, gen := current(t, m, "sota"); got == "" || gen == 0 {
		t.Fatalf("state damaged by concurrency: current=%q generation=%d", got, gen)
	}
}
