package knowledge

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"artix/pkg/policy"
	"artix/pkg/spec"
)

func TestKnowledgeStore_CRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kritix-ki-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewStore(tempDir)

	ki := &KnowledgeItem{
		ID:           "ki-test-01",
		Title:        "Handling KMP Coroutines Timeout",
		Category:     CategoryDebugging,
		Context:      "Using withTimeoutOrNull in Kotlin Multiplatform",
		Breakthrough: "Always cancel child scopes to prevent lingering background coroutines",
		CreatedAt:    time.Now(),
	}

	if err := store.Save(ki); err != nil {
		t.Fatalf("failed to save knowledge item: %v", err)
	}

	retrieved, err := store.Get("ki-test-01")
	if err != nil {
		t.Fatalf("failed to get knowledge item: %v", err)
	}
	if retrieved.Title != ki.Title || retrieved.Category != CategoryDebugging {
		t.Errorf("mismatched retrieved item: %+v", retrieved)
	}

	list, err := store.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("unexpected list: %v (len %d)", list, len(list))
	}

	if err := store.Delete("ki-test-01"); err != nil {
		t.Fatalf("failed to delete item: %v", err)
	}

	listAfter, _ := store.List()
	if len(listAfter) != 0 {
		t.Errorf("expected empty list after deletion, got %d", len(listAfter))
	}
}

func TestLearningExtractor_ExtractFromConvergence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kritix-extractor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewStore(tempDir)
	extractor := NewLearningExtractor(store)

	storySpec := &spec.StorySpec{
		ID:           "STORY-202",
		Title:        "Implement Redis Cache",
		UserStory:    "Cache hot database reads",
		TestCommands: []string{"go test -v ./..."},
	}

	convInfo := ConvergenceInfo{
		Success:   true,
		RoundsRun: 3, // Multi-round convergence
	}

	ki, err := extractor.ExtractFromConvergence(storySpec, convInfo)
	if err != nil {
		t.Fatalf("failed to extract knowledge item: %v", err)
	}
	if ki == nil {
		t.Fatalf("expected extracted knowledge item, got nil")
	}

	if ki.Category != CategoryDebugging || ki.Title == "" {
		t.Errorf("unexpected knowledge item: %+v", ki)
	}
}

func TestRuleSynthesizer_FromReviewComment(t *testing.T) {
	syn := NewRuleSynthesizer()

	// Taboo rule test
	comment := "Never allow direct sqlite imports in the Android UI layer"
	rule, err := syn.SynthesizeFromComment(comment)
	if err != nil {
		t.Fatalf("failed to synthesize rule: %v", err)
	}

	if !rule.IsTaboo {
		t.Errorf("expected taboo rule for 'never' comment")
	}

	hasAndroid := false
	for _, role := range rule.TargetRoles {
		if role == "android_engineer" {
			hasAndroid = true
		}
	}
	if !hasAndroid {
		t.Errorf("expected android_engineer role in targets: %+v", rule.TargetRoles)
	}

	// Heuristic rule test
	heuristicComment := "Always verify timeout boundaries on remote auth API calls"
	hRule, err := syn.SynthesizeFromComment(heuristicComment)
	if err != nil {
		t.Fatalf("failed to synthesize heuristic rule: %v", err)
	}
	if hRule.IsTaboo {
		t.Errorf("expected non-taboo heuristic rule")
	}
}

func TestKnowledgeStore_EnterpriseIsolationAndSchemaEvolution(t *testing.T) {
	tempProject := t.TempDir()

	// 1. Enterprise mode disables global store contamination
	store := NewStore(tempProject)
	store.SetEnterpriseMode(true)

	if !store.IsEnterpriseMode() {
		t.Fatal("expected enterprise mode to be active")
	}

	ki1 := &KnowledgeItem{
		ID:           "ki-ent-01",
		Title:        "Enterprise Isolation Rule",
		Category:     CategoryArchitecture,
		Context:      "Tenant data separation",
		Breakthrough: "Strict project-level scoping",
	}

	if err := store.Save(ki1); err != nil {
		t.Fatalf("failed to save in enterprise mode: %v", err)
	}

	retrieved, err := store.Get("ki-ent-01")
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}
	if retrieved.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", CurrentSchemaVersion, retrieved.SchemaVersion)
	}
	if retrieved.ContentHash == "" {
		t.Errorf("expected computed content hash, got empty")
	}

	// 2. Deduplication check
	kiDuplicate := &KnowledgeItem{
		ID:           "ki-ent-02",
		Title:        "Enterprise Isolation Rule",
		Category:     CategoryArchitecture,
		Context:      "Tenant data separation",
		Breakthrough: "Strict project-level scoping",
	}
	if err := store.Save(kiDuplicate); err != nil {
		t.Fatalf("failed to save duplicate: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("failed to list items: %v", err)
	}
	// Content duplicate is deduplicated in list
	if len(list) != 1 {
		t.Errorf("expected 1 deduplicated item, got %d", len(list))
	}
}

func TestKnowledgeStore_PruneTTL(t *testing.T) {
	tempProject := t.TempDir()
	store := NewStore(tempProject)

	oldItem := &KnowledgeItem{
		ID:           "ki-old-01",
		Title:        "Old Outdated Knowledge",
		Category:     CategoryDebugging,
		Context:      "Legacy JDK 8 quirk",
		Breakthrough: "Upgrade compiler",
		CreatedAt:    time.Now().Add(-48 * time.Hour),
	}
	newItem := &KnowledgeItem{
		ID:           "ki-new-01",
		Title:        "Recent Architecture Learning",
		Category:     CategoryArchitecture,
		Context:      "KMP memory model",
		Breakthrough: "Use atomic references",
		CreatedAt:    time.Now(),
	}

	if err := store.Save(oldItem); err != nil {
		t.Fatalf("failed to save old item: %v", err)
	}
	if err := store.Save(newItem); err != nil {
		t.Fatalf("failed to save new item: %v", err)
	}

	pruned, err := store.Prune(24 * time.Hour)
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 item pruned, got %d", pruned)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != "ki-new-01" {
		t.Errorf("expected only new item to remain, got %+v", list)
	}
}

func TestComputeHash_NoDelimiterCollision(t *testing.T) {
	// Item A: Category="auth|jwt", Title="fix"
	itemA := &KnowledgeItem{
		Category:     KnowledgeCategory("auth|jwt"),
		Title:        "fix",
		Context:      "c",
		Breakthrough: "b",
	}

	// Item B: Category="auth", Title="jwt|fix"
	itemB := &KnowledgeItem{
		Category:     KnowledgeCategory("auth"),
		Title:        "jwt|fix",
		Context:      "c",
		Breakthrough: "b",
	}

	hashA := computeHash(itemA)
	hashB := computeHash(itemB)

	if hashA == hashB {
		t.Fatalf("hash collision detected across pipe delimiter: hashA=%s, hashB=%s", hashA, hashB)
	}
}

func TestSynthesizedRuleGovernanceAndApproval(t *testing.T) {
	syn := NewRuleSynthesizer()
	rule, err := syn.SynthesizeFromComment("Never call Thread.sleep directly on main thread")
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}

	// 1. New rule must default to Proposed
	if rule.Status != RuleStatusProposed {
		t.Errorf("expected initial status proposed, got %s", rule.Status)
	}
	if rule.IsActive() {
		t.Errorf("unapproved rule must not be active")
	}

	// 2. Bot self-approval must fail
	if err := rule.Approve("artix-agent"); err == nil {
		t.Errorf("expected error when bot tries to approve synthesized rule, got nil")
	}
	if err := rule.Approve("github-actions[bot]"); err == nil {
		t.Errorf("expected error when bot identity approves, got nil")
	}

	// 3. Human approval must succeed
	if err := rule.Approve("lead-architect@corp.internal"); err != nil {
		t.Fatalf("human approval failed: %v", err)
	}
	if rule.Status != RuleStatusApproved {
		t.Errorf("expected status approved, got %s", rule.Status)
	}
	if !rule.IsActive() {
		t.Errorf("approved non-expired rule must be active")
	}

	// 4. Expired rule must not be active
	past := time.Now().Add(-1 * time.Hour)
	rule.ExpiresAt = &past
	if rule.IsActive() {
		t.Errorf("expired rule must not be active")
	}
}

func TestRuleConflictDetection(t *testing.T) {
	r1 := &SynthesizedRule{
		IsTaboo:  true,
		RuleText: "Never use raw sqlite in ViewModel",
	}
	r2 := &SynthesizedRule{
		IsTaboo:  false,
		RuleText: "Always use raw sqlite helper for quick queries",
	}
	r3 := &SynthesizedRule{
		IsTaboo:  true,
		RuleText: "Never use coroutine channels without buffer",
	}

	if !DetectConflict(r1, r2) {
		t.Errorf("expected conflict between contradictory sqlite rules")
	}
	if DetectConflict(r1, r3) {
		t.Errorf("did not expect conflict between sqlite and coroutine rules")
	}
}

func TestR13_6_KnowledgeStore_EnterpriseMode_ViaPolicyIsEnterprise(t *testing.T) {
	policy.EnforceSignedPolicy()
	defer func() {
		policy.RequireSignedPolicyFlag = "false"
		policy.ResetCachedPolicy()
	}()
	policy.ResetCachedPolicy()

	t.Setenv("ARTIX_ENTERPRISE", "")
	t.Setenv("KRITIX_ENTERPRISE", "")

	if !policy.IsEnterprise() {
		t.Fatalf("policy.IsEnterprise() must be true when EnforceSignedPolicy() is active")
	}

	tempDir := t.TempDir()
	store := NewStore(tempDir)

	if !store.IsEnterpriseMode() {
		t.Errorf("R13-6 VIOLATION: NewStore must have enterpriseMode=true when policy.IsEnterprise() is true")
	}
	if store.globalDir != "" {
		t.Errorf("R13-6 VIOLATION: NewStore must disable globalDir when policy.IsEnterprise() is true, got: %q", store.globalDir)
	}
}

func TestKnowledgeStore_SaveConditional_OnlyOnModifications(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	ki := &KnowledgeItem{
		ID:           "ki-save-cond-01",
		Title:        "Conditional Save Test",
		Category:     CategoryDebugging,
		Context:      "Testing conditional persistence",
		Breakthrough: "Only write to disk when contents change",
		SessionCount: 0,
		CreatedAt:    time.Now().Truncate(time.Second),
	}

	if err := store.Save(ki); err != nil {
		t.Fatalf("failed initial save: %v", err)
	}

	targetFile := filepath.Join(tempDir, ".artix", "knowledge", "ki-save-cond-01.json")
	fi1, err := os.Stat(targetFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	// Save again with identical item: must NOT rewrite or update mtime
	if err := store.Save(ki); err != nil {
		t.Fatalf("failed second save: %v", err)
	}

	fi2, err := os.Stat(targetFile)
	if err != nil {
		t.Fatalf("failed to stat file after second save: %v", err)
	}

	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Errorf("kStore.Save was not conditional: ModTime changed without modifications (before: %v, after: %v)",
			fi1.ModTime(), fi2.ModTime())
	}

	time.Sleep(20 * time.Millisecond)

	// Modify item: must trigger write and update mtime
	ki.Breakthrough = "Changed breakthrough payload"
	if err := store.Save(ki); err != nil {
		t.Fatalf("failed third save: %v", err)
	}

	fi3, err := os.Stat(targetFile)
	if err != nil {
		t.Fatalf("failed to stat file after third save: %v", err)
	}

	if !fi3.ModTime().After(fi1.ModTime()) {
		t.Errorf("expected ModTime to advance after actual modification, got %v <= %v", fi3.ModTime(), fi1.ModTime())
	}
}

func TestKnowledgeStore_CompareAndSwapSessionCount(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	ki := &KnowledgeItem{
		ID:           "ki-cas-test-01",
		Title:        "CAS Test",
		Category:     CategoryDebugging,
		SessionCount: 3,
		CreatedAt:    time.Now(),
	}

	if err := store.Save(ki); err != nil {
		t.Fatalf("failed initial save: %v", err)
	}

	// CAS with incorrect expected count should fail and not change value
	ok, err := store.CompareAndSwapSessionCount("ki-cas-test-01", 2, 4)
	if err != nil {
		t.Fatalf("unexpected CAS error: %v", err)
	}
	if ok {
		t.Errorf("expected CAS to fail when expected count did not match")
	}

	reloaded, _ := store.Get("ki-cas-test-01")
	if reloaded.SessionCount != 3 {
		t.Errorf("expected session count to remain 3 after failed CAS, got %d", reloaded.SessionCount)
	}

	// CAS with matching expected count should succeed
	ok, err = store.CompareAndSwapSessionCount("ki-cas-test-01", 3, 4)
	if err != nil {
		t.Fatalf("unexpected CAS error: %v", err)
	}
	if !ok {
		t.Errorf("expected CAS to succeed when expected count matched")
	}

	reloaded, _ = store.Get("ki-cas-test-01")
	if reloaded.SessionCount != 4 {
		t.Errorf("expected session count to update to 4, got %d", reloaded.SessionCount)
	}
}

func TestKnowledgeStore_ConcurrentSessionCountIncrement_CAS(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStore(tempDir)

	ki := &KnowledgeItem{
		ID:           "ki-concurrent-inc-01",
		Title:        "Concurrent Inc Test",
		Category:     CategoryDebugging,
		SessionCount: 0,
		CreatedAt:    time.Now(),
	}
	if err := store.Save(ki); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	const numWorkers = 20
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			workerStore := NewStore(tempDir)
			newCount, err := workerStore.IncrementSessionCount("ki-concurrent-inc-01")
			if err != nil {
				t.Errorf("increment failed: %v", err)
			}
			if newCount <= 0 {
				t.Errorf("expected positive count from increment, got %d", newCount)
			}
		}()
	}

	wg.Wait()

	finalKI, err := store.Get("ki-concurrent-inc-01")
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}
	if finalKI.SessionCount != numWorkers {
		t.Fatalf("expected SessionCount to be %d after %d concurrent increments, got %d", numWorkers, numWorkers, finalKI.SessionCount)
	}
}


