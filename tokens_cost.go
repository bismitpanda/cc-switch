package main

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed claude_pricing.json
var claudePricingJSON []byte

type modelPricingRaw struct {
	Input      float64  `json:"input"`
	Output     float64  `json:"output"`
	CacheWrite *float64 `json:"cache_write"`
	CacheRead  *float64 `json:"cache_read"`
}

type modelPricing struct {
	Input       float64
	Output      float64
	CacheCreate float64
	CacheRead   float64
}

const (
	cacheCreate1hMultiplier = 2.0
	defaultTierThreshold    = 200_000
)

var (
	pricingOnce sync.Once
	pricingMap  map[string]modelPricing
)

func loadClaudePricing() map[string]modelPricing {
	pricingOnce.Do(func() {
		raw := map[string]modelPricingRaw{}
		if err := json.Unmarshal(claudePricingJSON, &raw); err != nil {
			fatalf("embedded claude pricing: %v", err)
		}
		pricingMap = make(map[string]modelPricing, len(raw))
		for name, p := range raw {
			mp := modelPricing{
				Input:  p.Input / 1_000_000,
				Output: p.Output / 1_000_000,
			}
			if p.CacheWrite != nil {
				mp.CacheCreate = *p.CacheWrite / 1_000_000
			} else {
				mp.CacheCreate = mp.Input * 1.25
			}
			if p.CacheRead != nil {
				mp.CacheRead = *p.CacheRead / 1_000_000
			} else {
				mp.CacheRead = mp.Input * 0.1
			}
			pricingMap[name] = mp
		}
	})
	return pricingMap
}

func findModelPricing(model string) (modelPricing, bool) {
	if model == "" || model == "<synthetic>" {
		return modelPricing{}, false
	}
	m := loadClaudePricing()
	if p, ok := m[model]; ok {
		return p, true
	}
	for _, suffix := range []string{"-thinking", "-think", "-fast"} {
		if strings.HasSuffix(model, suffix) {
			base := strings.TrimSuffix(model, suffix)
			if p, ok := m[base]; ok {
				return p, true
			}
		}
	}
	if i := strings.LastIndex(model, "-"); i > 0 {
		tail := model[i+1:]
		if len(tail) == 8 && isAllDigits(tail) {
			if p, ok := m[model[:i]]; ok {
				return p, true
			}
		}
	}
	return modelPricing{}, false
}

func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

func tieredCost(tokens uint64, base float64, threshold uint64) float64 {
	if tokens == 0 {
		return 0
	}
	return float64(tokens) * base
}

func calculateCostFromTokens(model string, u tokenUsage) float64 {
	p, ok := findModelPricing(model)
	if !ok {
		return 0
	}
	cache5m, cache1h := u.cacheCreateSplit()
	cache1hRate := p.Input * cacheCreate1hMultiplier
	return tieredCost(u.InputTokens, p.Input, defaultTierThreshold) +
		tieredCost(u.OutputTokens, p.Output, defaultTierThreshold) +
		tieredCost(cache5m, p.CacheCreate, defaultTierThreshold) +
		tieredCost(cache1h, cache1hRate, defaultTierThreshold) +
		tieredCost(u.CacheReadTokens, p.CacheRead, defaultTierThreshold)
}

func resolveEntryCost(model string, u tokenUsage, costUSD *float64) float64 {
	if costUSD != nil {
		return *costUSD
	}
	return calculateCostFromTokens(model, u)
}
