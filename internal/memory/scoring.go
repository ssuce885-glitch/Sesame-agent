package memory

import (
	"math"
	"time"

	"go-agent/internal/types"
)

const DefaultHalfLife = 30 * 24 * time.Hour
const DeprecationThreshold = 0.25

func DecayFactor(age time.Duration, halfLife time.Duration) float64 {
	if halfLife <= 0 || age <= 0 {
		return 1.0
	}
	return math.Exp(-math.Ln2 * float64(age) / float64(halfLife))
}

func ComputeConfidence(entry types.MemoryEntry, now time.Time) float64 {
	base := 0.80
	switch entry.Kind {
	case types.MemoryKindWorkspaceOverview:
		base = 0.85
	case types.MemoryKindGlobalPreference:
		base = 0.90
	}

	usageBonus := math.Min(0.10, float64(entry.UsageCount)*0.02)

	recencyBonus := 0.0
	if !entry.LastUsedAt.IsZero() {
		recencyAge := now.Sub(entry.LastUsedAt)
		switch {
		case recencyAge <= 7*24*time.Hour:
			recencyBonus = 0.05
		case recencyAge <= 30*24*time.Hour:
			recencyBonus = 0.02
		}
	}

	agePenalty := 0.0
	if !entry.CreatedAt.IsZero() && now.Sub(entry.CreatedAt) > 30*24*time.Hour && entry.UsageCount == 0 {
		agePenalty = 0.10
	}

	return clamp(base+usageBonus+recencyBonus-agePenalty, 0.1, 1.0)
}

func EffectiveScore(entry types.MemoryEntry, now time.Time) float64 {
	age := now.Sub(entry.LastUsedAt)
	if entry.LastUsedAt.IsZero() {
		age = now.Sub(entry.CreatedAt)
	}
	if entry.LastUsedAt.IsZero() && entry.CreatedAt.IsZero() {
		age = 0
	}
	return ComputeConfidence(entry, now) * DecayFactor(age, DefaultHalfLife)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
