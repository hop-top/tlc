package core

// Unit coverage for the tag vocabulary and the two write paths that live
// in this package.
//
// The CLI and HTTP surfaces are covered end to end through the real
// binary in internal/cli/tag_policy_e2e_test.go. What is left for here is
// the plan importer, which has no CLI of its own to drive it, and the
// composition rules themselves, which are easier to state precisely
// against the vocabulary than through a subprocess.

import (
	"context"
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
)

// closedTagConfig returns a task config with a closed policy and the
// given project-specific vocabulary.
func closedTagConfig(allowed ...string) *config.TaskConfig {
	return &config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
		Tags: config.TagsConfig{
			Policy:  config.TagPolicyClosed,
			Allowed: allowed,
		},
	}
}

// TestTagPolicyDefaultsToOpen pins the default. A config that declares
// nothing about tags must yield the policy that changes nothing, which is
// what keeps this key from being a breaking change.
func TestTagPolicyDefaultsToOpen(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return builtinTaskConfig()
	})

	policy, _ := TagPolicyFor()
	if policy != config.TagPolicyOpen {
		t.Errorf("policy = %q, want %q", policy, config.TagPolicyOpen)
	}
	if err := ValidateTags([]string{"anything", "at:all", "even weird ones"}); err != nil {
		t.Errorf("open policy rejected a tag: %v", err)
	}
}

// TestClosedTagPolicyAdmitsGeneratedAxes is the composition contract.
func TestClosedTagPolicyAdmitsGeneratedAxes(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("storage")
	})

	admitted := []string{
		// Conventional Commits, mirrored from internal/labels.
		"type:feat", "type:fix", "type:breaking",
		// Built-in priorities, under BOTH spellings: their own names and
		// the rank aliases every sync plugin puts on the wire.
		"priority:p0", "priority:p3",
		"priority:critical", "priority:low",
		// Built-in efforts.
		"effort:xs", "effort:xl",
		// Statuses, plus the blocked marker that is not a status but is
		// on the axis because all four sync plugins push and pull it.
		"status:todo", "status:in-progress", "status:done", "status:blocked",
		// And the one thing config actually declared.
		"storage",
	}
	for _, tag := range admitted {
		if err := ValidateTags([]string{tag}); err != nil {
			t.Errorf("generated axis tag %q was rejected: %v", tag, err)
		}
	}
}

// TestClosedTagPolicyTracksRenamedVocabularies pins that composition
// reads the user's config rather than the built-ins. A hardcoded axis
// list would pass the test above and still get this wrong.
func TestClosedTagPolicyTracksRenamedVocabularies(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		cfg := closedTagConfig()
		cfg.Priorities = []config.PriorityDefinition{
			{Name: "URGENT"}, {Name: "LATER"},
		}
		cfg.Efforts = []config.EffortDefinition{{Name: "TINY"}, {Name: "BIG"}}
		cfg.Statuses = []config.StatusDefinition{
			{Name: "OPEN", Role: config.RoleInitial, TLSMarker: " "},
			{Name: "DOING", Role: config.RoleActive, TLSMarker: ">"},
			{Name: "SHIPPED", Role: "completed", IsTerminal: true, TLSMarker: "x"},
		}
		cfg.StateMachine = &config.WorkflowDefinition{
			Rules: map[string][]string{"OPEN": {"DOING"}, "DOING": {"SHIPPED"}},
		}
		return cfg
	})

	for _, tag := range []string{
		"priority:urgent", "priority:later",
		"effort:tiny", "effort:big",
		"status:open", "status:doing", "status:shipped",
	} {
		if err := ValidateTags([]string{tag}); err != nil {
			t.Errorf("renamed vocabulary's axis tag %q was rejected: %v", tag, err)
		}
	}

	// The abandoned built-ins must NOT be admitted: a vocabulary that
	// quietly kept them would advertise a set the config no longer has.
	for _, tag := range []string{"priority:p0", "effort:xs", "status:todo"} {
		if err := ValidateTags([]string{tag}); err == nil {
			t.Errorf("built-in axis tag %q admitted under a renamed vocabulary", tag)
		}
	}
}

// TestClosedTagPolicyUnderscoreStatusBecomesHyphen pins the axis-value
// convention: config names are shouty and underscored, axis values are
// lowercase and hyphenated. The two must agree or `sync push` sends a
// label the local policy would have rejected.
func TestClosedTagPolicyUnderscoreStatusBecomesHyphen(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig()
	})

	if err := ValidateTags([]string{"status:in-progress"}); err != nil {
		t.Errorf("IN_PROGRESS should yield status:in-progress: %v", err)
	}
	if err := ValidateTags([]string{"status:in_progress"}); err == nil {
		t.Error("status:in_progress should not be admitted; the axis hyphenates")
	}
}

// TestTagWildcardOpensOneDimension is the wildcard contract, and its
// bound. Opening a dimension must not open anything else, or `closed`
// would be a policy one config line could silently turn back into `open`.
func TestTagWildcardOpensOneDimension(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("domain:*")
	})

	for _, tag := range []string{"domain:storage", "domain:x", "DOMAIN:Api"} {
		if err := ValidateTags([]string{tag}); err != nil {
			t.Errorf("`domain:*` should admit %q: %v", tag, err)
		}
	}
	for _, tag := range []string{
		"team:core",         // a different dimension
		"domain:",           // the bare prefix is not a value
		"sub-domain:x",      // merely containing the prefix is not being anchored to it
		"predomain:storage", // nor is a longer prefix
	} {
		if err := ValidateTags([]string{tag}); err == nil {
			t.Errorf("`domain:*` must not admit %q", tag)
		}
	}
}

// TestTagLiteralClosesDimension is the other half of the wildcard
// decision: listing members literally keeps the dimension closed, so both
// guarantees are available and which applies is visible in config.
func TestTagLiteralClosesDimension(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("area:storage")
	})

	if err := ValidateTags([]string{"area:storage"}); err != nil {
		t.Errorf("a listed member was rejected: %v", err)
	}
	if err := ValidateTags([]string{"area:cli"}); err == nil {
		t.Error("a literal list must not admit an unlisted member of the same dimension")
	}
}

// TestTagRejectionNamesTagAndSet is the error-quality contract. A bare
// rejection is unusable when the vocabulary lives in a config file the
// user may not have written and cannot see from the error.
func TestTagRejectionNamesTagAndSet(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("storage", "domain:*")
	})

	err := ValidateTags([]string{"bogustag"})
	if err == nil {
		t.Fatal("an unlisted tag should be rejected")
	}
	msg := err.Error()
	for _, want := range []string{"bogustag", "storage", "domain:*", "type:feat"} {
		if !strings.Contains(msg, want) {
			t.Errorf("rejection should name %q, got: %s", want, msg)
		}
	}
}

// TestTagRejectionNamesEveryOffender pins that one call tells the user
// everything to fix, rather than making them discover the rejections one
// command at a time.
func TestTagRejectionNamesEveryOffender(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("storage")
	})

	err := ValidateTags([]string{"storage", "badone", "badtwo"})
	if err == nil {
		t.Fatal("two unlisted tags should be rejected")
	}
	for _, want := range []string{"badone", "badtwo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("rejection should name %q, got: %s", want, err)
		}
	}
	if strings.Contains(err.Error(), `"storage" not allowed`) {
		t.Errorf("rejection named an ALLOWED tag as an offender: %s", err)
	}
}

// TestTagVocabularyDisplayIsStableAndDeduped pins that the same config
// always produces the same message, and that a user who did restate a
// generated axis does not see it twice.
func TestTagVocabularyDisplayIsStableAndDeduped(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("type:feat", "storage")
	})

	_, vocab := TagPolicyFor()
	first := strings.Join(vocab.Display(), ",")
	_, vocab2 := TagPolicyFor()
	if second := strings.Join(vocab2.Display(), ","); first != second {
		t.Errorf("display is unstable:\n%s\n%s", first, second)
	}
	if n := strings.Count(first, "type:feat"); n != 1 {
		t.Errorf("restated axis appears %d times, want 1: %s", n, first)
	}
}

// TestCreateTasksFromPlanRejectsDisallowedTag covers the plan importer,
// which has no CLI surface of its own.
//
// A hard error rather than a filter, unlike the todo.txt ingest: a plan
// import is something the user asked for by name, so a tag the policy
// rejects is a plan to fix rather than noise to drop.
func TestCreateTasksFromPlanRejectsDisallowedTag(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("storage")
	})

	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")
	taskRepo := &creatingTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)

	specs := []PlanTaskSpec{
		{Title: "A", Tags: []string{"storage"}},
		{Title: "B", Tags: []string{"bogustag"}},
	}

	_, err := svc.CreateTasksFromPlan(context.Background(), "trk", specs, "", &stubIDGen{next: 400})
	if err == nil {
		t.Fatal("a plan carrying a disallowed tag should be rejected")
	}
	for _, want := range []string{"bogustag", "storage", `plan task 1`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %q, got: %v", want, err)
		}
	}

	// Nothing may be created: the check is a PREFLIGHT, so a rejected
	// plan must leave no prefix of itself behind.
	if len(taskRepo.created) != 0 {
		t.Errorf("rejected plan created %d tasks, want 0", len(taskRepo.created))
	}
}

// TestReconcileTasksFromPlanRejectsDisallowedTag covers the other half
// of the plan importer, and the one a create-only gate would miss:
// applySpecToTask assigns spec.Tags over whatever the task carried, so an
// unchecked reconcile is how a disallowed tag reaches an EXISTING task.
func TestReconcileTasksFromPlanRejectsDisallowedTag(t *testing.T) {
	original := []PlanTaskSpec{{Title: "Task A", Tags: []string{"storage"}}}

	// Set up while the policy is open, so the fixture is not itself
	// gated by the thing under test.
	svc, taskRepo, mapping := setupTrackWithTasks(t, original)
	ctx := context.Background()

	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("storage")
	})

	updated := []PlanTaskSpec{{Title: "Task A", Tags: []string{"bogustag"}}}
	_, err := svc.ReconcileTasksFromPlan(ctx, "test-track", updated, "", taskRepo, mapping)
	if err == nil {
		t.Fatal("reconcile with a disallowed tag should be rejected")
	}
	if !strings.Contains(err.Error(), "bogustag") {
		t.Errorf("error should name the offending tag, got: %v", err)
	}

	// The existing task must be untouched: the check is a preflight over
	// EVERY spec, so a rejected plan leaves nothing half-reconciled.
	task, _ := taskRepo.GetTask(ctx, mapping[0])
	if task == nil || len(task.Tags) != 1 || task.Tags[0] != "storage" {
		t.Errorf("existing task was modified by a rejected reconcile: %+v", task)
	}
}

// TestCreateTasksFromPlanAcceptsAllowedTags is the positive half, and
// the no-regression guard for an open policy.
func TestCreateTasksFromPlanAcceptsAllowedTags(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  func() *config.TaskConfig
		tags []string
	}{
		{"closed, listed", func() *config.TaskConfig { return closedTagConfig("storage") }, []string{"storage"}},
		{"closed, axis", func() *config.TaskConfig { return closedTagConfig() }, []string{"type:feat"}},
		{"open, anything", builtinTaskConfig, []string{"whatever"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withTaskConfigProvider(t, tc.cfg)

			trackRepo := newStubTrackRepo()
			ensureTrack(t, trackRepo, "trk")
			taskRepo := &creatingTaskRepo{}
			svc := NewTrackService(trackRepo, taskRepo)

			specs := []PlanTaskSpec{{Title: "A", Tags: tc.tags}}
			if _, err := svc.CreateTasksFromPlan(
				context.Background(), "trk", specs, "", &stubIDGen{next: 500},
			); err != nil {
				t.Fatalf("plan with tags %v rejected: %v", tc.tags, err)
			}
			if len(taskRepo.created) != 1 {
				t.Fatalf("created %d tasks, want 1", len(taskRepo.created))
			}
		})
	}
}

// TestSuggestedTagsComposesUnderOpenPolicy pins the deliberate
// difference from TagPolicyFor, which returns an EMPTY vocabulary under
// `open` because nothing enforces membership there. A chooser wants the
// set anyway: they are the tags `label init` and `sync` emit.
func TestSuggestedTagsComposesUnderOpenPolicy(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return builtinTaskConfig()
	})

	_, vocab := TagPolicyFor()
	if len(vocab.Display()) != 0 {
		t.Fatalf("precondition: open policy should build no vocabulary, got %v",
			vocab.Display())
	}

	got := SuggestedTags()
	if len(got) == 0 {
		t.Fatal("SuggestedTags returned nothing under an open policy")
	}
	for _, want := range []string{"type:feat", "status:todo", "priority:p0"} {
		if !containsTag(got, want) {
			t.Errorf("composed axis %q missing: %v", want, got)
		}
	}
}

// TestSuggestedTagsExcludesWildcardOpeners: `domain:*` widens a
// namespace, it is not a taggable value. Offering the literal would
// seed a tag the policy that declared it rejects.
func TestSuggestedTagsExcludesWildcardOpeners(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("domain:*", "storage")
	})

	got := SuggestedTags()
	if containsTag(got, "domain:*") {
		t.Errorf("wildcard opener suggested as a literal tag: %v", got)
	}
	if !containsTag(got, "storage") {
		t.Errorf("declared literal missing: %v", got)
	}
	if err := ValidateTags(got); err != nil {
		t.Errorf("every suggestion must pass the gate it was built from: %v", err)
	}
}

// TestSuggestedTagsFollowsRenamedVocabulary proves composition rather
// than a second hardcoded list one rename behind.
func TestSuggestedTagsFollowsRenamedVocabulary(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return &config.TaskConfig{
			Statuses:     config.GetDefaultStatuses(),
			StateMachine: config.GetDefaultStateMachine(),
			Priorities: []config.PriorityDefinition{
				{Name: "URGENT"}, {Name: "NORMAL"}, {Name: "LATER"},
			},
		}
	})

	got := SuggestedTags()
	for _, want := range []string{"priority:urgent", "priority:normal", "priority:later"} {
		if !containsTag(got, want) {
			t.Errorf("renamed priority axis %q missing: %v", want, got)
		}
	}
	if containsTag(got, "priority:p0") {
		t.Errorf("built-in priority axis leaked under a renamed vocabulary: %v", got)
	}
}

func containsTag(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// TestClosedTagPolicyAdmitsSeededNeedsAndDomain covers the tags
// internal/labels seeds for EVERY project type but the composition did
// not name.
//
// `label init` writes `needs:triage` and a `domain:*` set to the forge,
// and github-sync's mapLabelsToTask sends both back as task tags. A
// closed policy that omitted them stripped a tag tlc had just told the
// forge to use — the label survived the mapper's colon check and died one
// layer later, at this gate.
func TestClosedTagPolicyAdmitsSeededNeedsAndDomain(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig("storage")
	})

	// Driven off the axis itself rather than a retyped copy: a test that
	// restated the three values would keep passing if the axis lost one.
	admitted := append([]string(nil), needsTags...)
	admitted = append(admitted,
		// `domain:*` is per project TYPE, which core cannot see, so the
		// axis is opened rather than enumerated. Two values from
		// different templates, to pin that it is the namespace that is
		// open and not one template's list.
		"domain:cli", "domain:terraform",
	)
	for _, tag := range admitted {
		if err := ValidateTags([]string{tag}); err != nil {
			t.Errorf("seeded label tag %q was rejected: %v", tag, err)
		}
	}

	// The opener widens exactly one namespace and no further: a bare
	// `domain` with nothing after the colon is not a value.
	if err := ValidateTags([]string{"domain"}); err == nil {
		t.Error("bare \"domain\" was admitted; the opener is not a value")
	}
	if err := ValidateTags([]string{"needs:sleep"}); err == nil {
		t.Error("needs:* is a literal set, not an open namespace")
	}
}

// TestSuggestedTagsOffersSeededNeedsWithoutDomainOpener pins the chooser
// half of the same composition. `needs:triage` is a value a user should
// be offered; `domain:*` is an opener, and offering the literal string
// would seed a tag the policy that declared it rejects.
func TestSuggestedTagsOffersSeededNeedsWithoutDomainOpener(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return closedTagConfig()
	})

	suggested := SuggestedTags()
	joined := strings.Join(suggested, " ")
	for _, want := range needsTags {
		if !strings.Contains(joined, want) {
			t.Errorf("SuggestedTags omitted %q: %v", want, suggested)
		}
	}
	for _, tag := range suggested {
		if tag == domainTagPrefix {
			t.Errorf("SuggestedTags offered the wildcard opener %q", tag)
		}
	}
}

// TestDomainWildcardHeldForUnseededConfig is the compatibility half of
// the wildcard narrowing. A config that declared `closed` before `label
// init` learned to seed has no `domain:` entry, and must keep admitting
// the whole namespace: withdrawing the wildcard from it would start
// rejecting tags it accepted yesterday, on a config its owner never
// touched.
func TestDomainWildcardHeldForUnseededConfig(t *testing.T) {
	vocab := buildTagVocabulary(closedTagConfig("area:billing"))

	for _, tag := range []string{"domain:cli", "domain:strage", "domain:anything"} {
		if !vocab.Admits(tag) {
			t.Errorf("unseeded closed config rejected %q; the wildcard must still apply", tag)
		}
	}
}

// TestDomainWildcardWithdrawnOnceEnumerated is the closure half. Once
// the config names its own domains, the namespace is no longer open: a
// seeded value is admitted and a typo of one is not. This is the whole
// point of recording the seeded values.
func TestDomainWildcardWithdrawnOnceEnumerated(t *testing.T) {
	vocab := buildTagVocabulary(closedTagConfig(
		"domain:cli", "domain:core", "domain:storage",
	))

	for _, tag := range []string{"domain:cli", "domain:core", "domain:storage"} {
		if !vocab.Admits(tag) {
			t.Errorf("seeded config rejected its own domain %q", tag)
		}
	}
	for _, tag := range []string{"domain:strage", "domain:k8s", "domain:nope"} {
		if vocab.Admits(tag) {
			t.Errorf("seeded config admitted unenumerated %q; closure not regained", tag)
		}
	}
}

// TestExplicitDomainWildcardStillOpensNamespace pins the escape hatch. A
// user who wrote `domain:*` themselves asked for an open namespace, and
// the entry carries the wildcard suffix rather than a literal value, so
// it must not be read as an enumeration that withdraws the opener.
func TestExplicitDomainWildcardStillOpensNamespace(t *testing.T) {
	vocab := buildTagVocabulary(closedTagConfig("domain:*"))

	for _, tag := range []string{"domain:cli", "domain:whatever"} {
		if !vocab.Admits(tag) {
			t.Errorf("explicit domain:* did not open the namespace for %q", tag)
		}
	}
}

// TestEnumeratedDomainsLeaveOtherAxesAlone guards the blast radius. The
// narrowing is scoped to the domain axis: withdrawing its opener must
// not disturb the generated axes or the `needs:*` set, which no config
// enumerates and every sync plugin writes.
func TestEnumeratedDomainsLeaveOtherAxesAlone(t *testing.T) {
	vocab := buildTagVocabulary(closedTagConfig("domain:cli"))

	for _, tag := range []string{
		"type:feat", "priority:high", "effort:m",
		"status:in-progress", "status:blocked",
		"needs:triage", "needs:repro", "needs:decision",
	} {
		if !vocab.Admits(tag) {
			t.Errorf("enumerating domains wrongly rejected %q on another axis", tag)
		}
	}
	if vocab.Admits("wildly:invalid") {
		t.Error("closed policy admitted a tag outside every axis")
	}
}

// TestDeclaresDomainLiterals states the condition directly, so the rule
// that decides whether the wildcard applies is readable without building
// a whole vocabulary to infer it.
func TestDeclaresDomainLiterals(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		want    bool
	}{
		{"empty", nil, false},
		{"unrelated only", []string{"area:billing"}, false},
		{"wildcard only", []string{"domain:*"}, false},
		{"literal", []string{"domain:cli"}, true},
		{"literal beside wildcard", []string{"domain:*", "domain:cli"}, true},
		{"mixed case literal", []string{"Domain:CLI"}, true},
		{"padded literal", []string{"  domain:core  "}, true},
		{"bare axis with no value", []string{"domain:"}, false},
		{"other axis wildcard", []string{"area:*"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := declaresDomainLiterals(tc.allowed); got != tc.want {
				t.Errorf("declaresDomainLiterals(%v) = %v, want %v",
					tc.allowed, got, tc.want)
			}
		})
	}
}
