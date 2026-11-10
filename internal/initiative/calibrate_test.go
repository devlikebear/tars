package initiative

import (
	"math"
	"testing"
)

func jevRecord(source string, probs map[string]float64, label DatasetLabel) DatasetRecord {
	return DatasetRecord{Text: TextSignals{Source: source, Probabilities: probs}, Label: label}
}

func TestCalibrationPointsSkipsCacheAndUnlabeled(t *testing.T) {
	records := []DatasetRecord{
		jevRecord("systemone", map[string]float64{"quiet_requested": 0.9, "user_strained": 0.2}, DatasetLabel{QuietRequested: boolPtr(true)}),
		jevRecord("cache", map[string]float64{"quiet_requested": 0.9}, DatasetLabel{QuietRequested: boolPtr(true)}),
		jevRecord("llm", nil, DatasetLabel{QuietRequested: boolPtr(true)}),
		jevRecord("systemone", map[string]float64{"special_day": 0.1}, DatasetLabel{SpecialDay: boolPtr(false), UserStrained: boolPtr(true)}),
	}
	got := CalibrationPoints(records)
	if len(got[QuestionQuietRequested]) != 1 || len(got[QuestionUserStrained]) != 0 || len(got[QuestionSpecialDay]) != 1 {
		t.Fatalf("points = %+v", got)
	}
}

func TestCalibrateThresholdFindsSeparatingCut(t *testing.T) {
	points := []CalibrationPoint{
		{0.10, false}, {0.20, false}, {0.30, false}, {0.40, false},
		{0.45, true}, {0.70, true}, {0.90, true},
	}
	rep := CalibrateThreshold(QuestionQuietRequested, points, 0.55, 2)
	if rep.Samples != 7 || rep.Positives != 3 || rep.AUC != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Current.FN != 1 || rep.Current.TP != 2 || rep.Current.TN != 4 {
		t.Fatalf("current = %+v", rep.Current)
	}
	if rep.Candidate == nil || rep.Candidate.Threshold != 0.425 || rep.Candidate.BalancedAccuracy != 1 {
		t.Fatalf("candidate = %+v", rep.Candidate)
	}
}

func TestCalibrateThresholdTieKeepsNearestCurrent(t *testing.T) {
	// 0.35 and 0.65 both split these fully overlapping points equally
	// badly; the cut nearest the current threshold wins.
	points := []CalibrationPoint{{0.2, true}, {0.2, false}, {0.5, true}, {0.5, false}, {0.8, true}, {0.8, false}}
	rep := CalibrateThreshold(QuestionUserStrained, points, 0.6, 1)
	if rep.Candidate == nil || rep.Candidate.Threshold != 0.65 {
		t.Fatalf("candidate = %+v", rep.Candidate)
	}
	if math.Abs(rep.AUC-0.5) > 1e-9 {
		t.Fatalf("auc = %v", rep.AUC)
	}
}

func TestCalibrateThresholdNeedsBothClasses(t *testing.T) {
	rep := CalibrateThreshold(QuestionSpecialDay, []CalibrationPoint{{0.9, true}, {0.8, true}, {0.1, false}}, 0.55, 2)
	if rep.Candidate != nil || rep.AUC != 1 {
		t.Fatalf("report = %+v", rep)
	}
	empty := CalibrateThreshold(QuestionSpecialDay, nil, 0.55, 0)
	if empty.Candidate != nil || empty.AUC != 0 || empty.Current.BalancedAccuracy != 0 {
		t.Fatalf("empty = %+v", empty)
	}
}

func TestCalibrateThresholdsReportsAllQuestions(t *testing.T) {
	reps := CalibrateThresholds(nil, Thresholds{QuietRequested: 0.55, UserStrained: 0.6, SpecialDay: 0.55}, 5)
	if len(reps) != 3 || reps[1].Question != QuestionUserStrained || reps[1].Current.Threshold != 0.6 {
		t.Fatalf("reports = %+v", reps)
	}
}
