package initiative

import (
	"math"
	"sort"
)

// Text-signal question names, as they appear in TextSignals.Probabilities
// and in initiative.thresholds.<name>.
const (
	QuestionQuietRequested = "quiet_requested"
	QuestionUserStrained   = "user_strained"
	QuestionSpecialDay     = "special_day"
)

// CalibrationPoint is one jev probability paired with the user's label for
// the same reading.
type CalibrationPoint struct {
	P     float64
	Truth bool
}

// ThresholdScore is how a threshold (probability >= Threshold means true)
// classifies a set of labeled points.
type ThresholdScore struct {
	Threshold        float64 `json:"threshold"`
	TP               int     `json:"tp"`
	FP               int     `json:"fp"`
	TN               int     `json:"tn"`
	FN               int     `json:"fn"`
	BalancedAccuracy float64 `json:"balanced_accuracy"`
}

// ThresholdReport is the offline calibration of one question's threshold
// (tars#1003). Candidate is nil when there are too few labeled readings of
// either class to say anything; the current threshold is then kept.
type ThresholdReport struct {
	Question  string          `json:"question"`
	Samples   int             `json:"samples"`
	Positives int             `json:"positives"`
	AUC       float64         `json:"auc"`
	Current   ThresholdScore  `json:"current"`
	Candidate *ThresholdScore `json:"candidate,omitempty"`
}

// CalibrationPoints collects, per question, the probability a jev reading
// reported and the label the user gave it. Only fresh jev readings count:
// a "cache" record repeats an earlier reading of the same text and would
// weigh it twice, and the llm backend reports no probabilities at all.
func CalibrationPoints(records []DatasetRecord) map[string][]CalibrationPoint {
	out := map[string][]CalibrationPoint{}
	for _, r := range records {
		if r.Text.Source == "cache" || len(r.Text.Probabilities) == 0 {
			continue
		}
		for name, label := range map[string]*bool{
			QuestionQuietRequested: r.Label.QuietRequested,
			QuestionUserStrained:   r.Label.UserStrained,
			QuestionSpecialDay:     r.Label.SpecialDay,
		} {
			p, ok := r.Text.Probabilities[name]
			if !ok || label == nil {
				continue
			}
			out[name] = append(out[name], CalibrationPoint{P: p, Truth: *label})
		}
	}
	return out
}

// CalibrateThresholds scores the current thresholds against labeled jev
// readings and proposes, per question, the threshold with the best
// balanced accuracy. It never changes configuration; the caller prints
// the result.
func CalibrateThresholds(records []DatasetRecord, current Thresholds, minPerClass int) []ThresholdReport {
	points := CalibrationPoints(records)
	return []ThresholdReport{
		CalibrateThreshold(QuestionQuietRequested, points[QuestionQuietRequested], current.QuietRequested, minPerClass),
		CalibrateThreshold(QuestionUserStrained, points[QuestionUserStrained], current.UserStrained, minPerClass),
		CalibrateThreshold(QuestionSpecialDay, points[QuestionSpecialDay], current.SpecialDay, minPerClass),
	}
}

// CalibrateThreshold picks the threshold that maximizes balanced accuracy
// (the mean of the true-positive and true-negative rates — positives are
// rare, so plain accuracy would favor never firing). Candidates are the
// midpoints between consecutive distinct probabilities, since every
// threshold between two observed values classifies the points alike. A
// tie goes to the candidate nearest the current threshold, so a dataset
// that cannot tell two thresholds apart does not move it. With fewer than
// minPerClass labeled readings of either class there is no candidate.
func CalibrateThreshold(question string, points []CalibrationPoint, current float64, minPerClass int) ThresholdReport {
	rep := ThresholdReport{Question: question, Samples: len(points), Current: scoreThreshold(points, current)}
	for _, p := range points {
		if p.Truth {
			rep.Positives++
		}
	}
	rep.AUC = rocAUC(points)
	if minPerClass < 1 {
		minPerClass = 1
	}
	if rep.Positives < minPerClass || rep.Samples-rep.Positives < minPerClass {
		return rep
	}
	probs := make([]float64, 0, len(points))
	for _, p := range points {
		probs = append(probs, p.P)
	}
	sort.Float64s(probs)
	var best *ThresholdScore
	for i := 1; i < len(probs); i++ {
		if probs[i] == probs[i-1] {
			continue
		}
		th := math.Round((probs[i]+probs[i-1])/2*1000) / 1000
		s := scoreThreshold(points, th)
		if best == nil || s.BalancedAccuracy > best.BalancedAccuracy ||
			(s.BalancedAccuracy == best.BalancedAccuracy && math.Abs(th-current) < math.Abs(best.Threshold-current)) {
			best = &s
		}
	}
	rep.Candidate = best
	return rep
}

func scoreThreshold(points []CalibrationPoint, th float64) ThresholdScore {
	s := ThresholdScore{Threshold: th}
	for _, p := range points {
		predicted := p.P >= th
		switch {
		case predicted && p.Truth:
			s.TP++
		case predicted:
			s.FP++
		case p.Truth:
			s.FN++
		default:
			s.TN++
		}
	}
	var tpr, tnr float64
	if s.TP+s.FN > 0 {
		tpr = float64(s.TP) / float64(s.TP+s.FN)
	}
	if s.TN+s.FP > 0 {
		tnr = float64(s.TN) / float64(s.TN+s.FP)
	}
	s.BalancedAccuracy = (tpr + tnr) / 2
	return s
}

// rocAUC is the probability that a random positive scores above a random
// negative (ties count half) — the measure the P0 spike (tars#998) used.
// It is 0 when either class is missing.
func rocAUC(points []CalibrationPoint) float64 {
	var pos, neg []float64
	for _, p := range points {
		if p.Truth {
			pos = append(pos, p.P)
		} else {
			neg = append(neg, p.P)
		}
	}
	if len(pos) == 0 || len(neg) == 0 {
		return 0
	}
	var wins float64
	for _, a := range pos {
		for _, b := range neg {
			switch {
			case a > b:
				wins++
			case a == b:
				wins += 0.5
			}
		}
	}
	return wins / float64(len(pos)*len(neg))
}
