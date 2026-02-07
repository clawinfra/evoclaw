package router

import "encoding/json"

// Tier represents the model complexity tier.
type Tier int

const (
	TierSimple    Tier = iota // Cheap, fast — greetings, simple factual questions
	TierMedium                // Mid-range — summarisation, light code, moderate Q&A
	TierComplex               // Full capability — deep analysis, complex code, multi-step
	TierReasoning             // Specialised reasoning — math proofs, logic chains, planning
)

var tierNames = [...]string{"SIMPLE", "MEDIUM", "COMPLEX", "REASONING"}

func (t Tier) String() string {
	if int(t) < len(tierNames) {
		return tierNames[t]
	}
	return "UNKNOWN"
}

// MarshalJSON implements json.Marshaler.
func (t Tier) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *Tier) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// Try as integer
		var i int
		if err2 := json.Unmarshal(data, &i); err2 != nil {
			return err
		}
		*t = Tier(i)
		return nil
	}
	switch s {
	case "SIMPLE":
		*t = TierSimple
	case "MEDIUM":
		*t = TierMedium
	case "COMPLEX":
		*t = TierComplex
	case "REASONING":
		*t = TierReasoning
	default:
		*t = TierComplex // safe default
	}
	return nil
}

// SelectTier maps a normalised score [0,1] to a Tier using the configured thresholds.
func SelectTier(score float64, thresholds [3]float64) Tier {
	if score < thresholds[0] {
		return TierSimple
	}
	if score < thresholds[1] {
		return TierMedium
	}
	if score < thresholds[2] {
		return TierComplex
	}
	return TierReasoning
}
