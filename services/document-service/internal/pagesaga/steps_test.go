package pagesaga

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemainingResumesAtTheFirstUnrecordedStep(t *testing.T) {
	// The mockup's own claim (ui-mockups § 23c): "the process died mid-delete
	// at step 3 and resumed there, not from the start."
	got := Remaining([]string{StepTreeDetached, StepLinksRewritten})
	require.Equal(t, []string{
		StepSearchIndex, StepSessionsReleased, StepEmbeddingsPurged, StepBlobsReleased,
	}, got)
}

func TestRemainingIsOrderedByStepsNotByStepsDone(t *testing.T) {
	// steps_done is appended to in completion order, but a resume must run
	// what is left in SAGA order. Feeding it back out of order is not
	// hypothetical: a step that ran twice (effect landed, record didn't) can
	// legitimately record itself after a later one.
	got := Remaining([]string{StepSearchIndex, StepTreeDetached})
	require.Equal(t, []string{
		StepLinksRewritten, StepSessionsReleased, StepEmbeddingsPurged, StepBlobsReleased,
	}, got)
}

func TestRemainingIsEmptyOnlyWhenEveryStepIsDone(t *testing.T) {
	require.Empty(t, Remaining(Steps))

	// One short is not finished — the guard against a saga completing because
	// its list happened to be long enough.
	require.Equal(t, []string{StepBlobsReleased}, Remaining(Steps[:len(Steps)-1]))
}

func TestRemainingIgnoresUnknownRecordedSteps(t *testing.T) {
	// A row written by a NEWER release naming a step this binary does not
	// have. It must not crash, and it must not cause the known steps to be
	// treated as done — a rolled-back deploy has to keep sweeping safely.
	got := Remaining([]string{StepTreeDetached, "steps_from_the_future"})
	require.Equal(t, []string{
		StepLinksRewritten, StepSearchIndex, StepSessionsReleased,
		StepEmbeddingsPurged, StepBlobsReleased,
	}, got)
}

func TestAppendingAStepReopensCompletedSagas(t *testing.T) {
	// Deliberate, and documented in steps.go: a page deleted before v4.4.0
	// genuinely does have embeddings to purge once embeddings exist. This
	// pins the behaviour so a future release cannot quietly "fix" it into
	// treating a short historical list as complete.
	historical := []string{
		StepTreeDetached, StepLinksRewritten, StepSearchIndex, StepSessionsReleased,
	}
	require.Equal(t, []string{StepEmbeddingsPurged, StepBlobsReleased}, Remaining(historical))
}

func TestNotApplicableNamesOnlyTheStepsWithNoStore(t *testing.T) {
	require.True(t, NotApplicable(StepEmbeddingsPurged))
	require.True(t, NotApplicable(StepBlobsReleased))

	// StepSearchIndex is a real step that currently does nothing, which is a
	// different claim from "not applicable" — the index exists, it is just
	// dropped by the row delete. Reporting it as n/a would be a lie to the
	// trash screen.
	require.False(t, NotApplicable(StepSearchIndex))
	require.False(t, NotApplicable(StepTreeDetached))
	require.False(t, NotApplicable(StepSessionsReleased))
}

func TestStepsHasNoDuplicates(t *testing.T) {
	// steps_done treats names as a set; a duplicate in Steps would make
	// Remaining skip the second occurrence the moment the first records.
	seen := map[string]bool{}
	for _, s := range Steps {
		require.False(t, seen[s], "duplicate step %q", s)
		seen[s] = true
	}
}

func TestResumedIsAttemptsGreaterThanOne(t *testing.T) {
	require.False(t, Resumed(1), "a first attempt has not resumed")
	require.True(t, Resumed(2))
}
