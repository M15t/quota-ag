// Package display provides terminal UI formatting for quota information
package display

import (
	"fmt"
	"sort"
	"strings"

	"quota-ag/internal/models"
)

// ANSI color codes
const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Dim    = "\033[90m"
	Cyan   = "\033[36m"
	Yellow = "\033[33m"
	Green  = "\033[32m"
	Red    = "\033[31m"
)

// modelDisplayNames maps model IDs to user-friendly display names
var modelDisplayNames = map[string]string{
	// Gemini models
	"MODEL_GOOGLE_GEMINI_COMPUTER_USE_EXPERIMENTAL": "Gemini Computer Use (Exp)",
	"MODEL_GOOGLE_GEMINI_2_5_FLASH":                 "Gemini 2.5 Flash",
	"MODEL_GOOGLE_GEMINI_2_5_FLASH_THINKING":        "Gemini 2.5 Flash (Thinking)",
	"MODEL_GOOGLE_GEMINI_2_5_FLASH_LITE":            "Gemini 2.5 Flash Lite",
	"MODEL_GOOGLE_GEMINI_2_5_PRO":                   "Gemini 2.5 Pro",
	// Claude models
	"MODEL_CLAUDE_4_5_SONNET":          "Claude Sonnet 4.5",
	"MODEL_CLAUDE_4_5_SONNET_THINKING": "Claude Sonnet 4.5 (Thinking)",
	// Placeholder models (new/experimental)
	"MODEL_PLACEHOLDER_M8":  "Gemini 3 Pro (High)",
	"MODEL_PLACEHOLDER_M12": "Claude Opus 4.5 (Thinking)",
	"MODEL_PLACEHOLDER_M18": "Gemini 3 Flash",
	"MODEL_PLACEHOLDER_M19": "Gemini 3 Flash (Exp)",
	// OpenAI models
	"MODEL_OPENAI_GPT_OSS_120B_MEDIUM": "GPT-OSS 120B (Medium)",
	// Chat models (unknown)
	"MODEL_CHAT_20706": "Chat Model 20706",
	"MODEL_CHAT_23310": "Chat Model 23310",
}

// modelProviders maps model name prefixes to provider names
var modelProviders = map[string]string{
	"Gemini":    "Google Gemini",
	"Claude":    "Anthropic Claude",
	"GPT":       "OpenAI",
	"Chat":      "Other",
	"Gpt":       "OpenAI",
	"Anthropic": "Anthropic Claude",
}

// providerOrder defines the display order for providers
var providerOrder = []string{"Google Gemini", "Anthropic Claude", "OpenAI", "Other"}

// GetModelDisplayName returns a display name for a model
func GetModelDisplayName(modelID, displayName string) string {
	// If the API provided a display name, use it
	if displayName != "" {
		return displayName
	}
	// Check our mapping
	if name, ok := modelDisplayNames[modelID]; ok {
		return name
	}
	// Fallback: format the model ID
	name := strings.TrimPrefix(modelID, "MODEL_")
	name = strings.ReplaceAll(name, "_", " ")
	// Title case each word
	words := strings.Fields(name)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}

// GetProvider returns the provider name for a model
func GetProvider(modelName string) string {
	for prefix, provider := range modelProviders {
		if strings.HasPrefix(modelName, prefix) {
			return provider
		}
	}
	return "Other"
}

// GroupModelsByProvider groups models by their provider
func GroupModelsByProvider(modelList []models.ModelQuota) map[string][]models.ModelQuota {
	groups := make(map[string][]models.ModelQuota, len(providerOrder))
	for _, m := range modelList {
		provider := GetProvider(m.Name)
		if groups[provider] == nil {
			groups[provider] = make([]models.ModelQuota, 0, 4)
		}
		groups[provider] = append(groups[provider], m)
	}
	return groups
}

// SortModels applies display names and sorts models alphabetically
func SortModels(quota *models.QuotaStatus) {
	// Apply display names before sorting
	for i := range quota.Models {
		if quota.Models[i].Name == "" {
			quota.Models[i].Name = GetModelDisplayName(quota.Models[i].ModelID, "")
		}
	}
	sort.Slice(quota.Models, func(i, j int) bool {
		return quota.Models[i].Name < quota.Models[j].Name
	})
}

// GetProviderIcon returns a Unicode icon for a provider
func GetProviderIcon(provider string) string {
	switch provider {
	case "Google Gemini":
		return "◆"
	case "Anthropic Claude":
		return "◇"
	case "OpenAI":
		return "○"
	default:
		return "◌"
	}
}

// ProgressBar creates a fancy progress bar with colors
func ProgressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	color := GetColor(pct)

	// Use different characters for filled/empty
	bar := color + strings.Repeat("━", filled) + Dim + strings.Repeat("─", width-filled) + Reset
	return bar
}

// ProgressBarSimple creates a simple progress bar without colors
func ProgressBarSimple(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return bar
}

// GetColor returns the ANSI color code based on percentage
func GetColor(pct float64) string {
	if pct <= 10 {
		return Red
	} else if pct <= 30 {
		return Yellow
	}
	return Green
}

// Truncate shortens a string to maxLen, adding ellipsis if needed
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// MaskEmail masks an email address for privacy
func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 && len(parts[0]) >= 5 {
		return parts[0][:5] + "...@" + parts[1]
	}
	return email
}

// PrintTable displays quota information in a formatted table
func PrintTable(quota *models.QuotaStatus) {
	if len(quota.Models) == 0 {
		fmt.Println("No models found.")
		return
	}

	// Calculate summary stats
	var lowQuota, criticalQuota int
	for _, m := range quota.Models {
		// Skip expired models
		if m.ResetIn == "expired" {
			continue
		}
		if m.RemainingPct <= 10 {
			criticalQuota++
		} else if m.RemainingPct <= 30 {
			lowQuota++
		}
	}

	// Header
	fmt.Println()
	fmt.Println("  " + Bold + Cyan + "ANTIGRAVITY QUOTA DASHBOARD" + Reset)
	if quota.Email != "" {
		fmt.Println("  " + Dim + MaskEmail(quota.Email) + Reset)
	}
	fmt.Println()

	// Summary line
	fmt.Printf("  %s%d models%s", Dim, len(quota.Models), Reset)
	if criticalQuota > 0 {
		fmt.Printf("  %s● %d critical%s", Red, criticalQuota, Reset)
	}
	if lowQuota > 0 {
		fmt.Printf("  %s● %d low%s", Yellow, lowQuota, Reset)
	}
	fmt.Println()
	fmt.Println("  " + Dim + strings.Repeat("─", 62) + Reset)

	// Group models by provider
	groups := GroupModelsByProvider(quota.Models)

	for _, provider := range providerOrder {
		providerModels, exists := groups[provider]
		if !exists || len(providerModels) == 0 {
			continue
		}

		// Provider header
		providerIcon := GetProviderIcon(provider)
		fmt.Printf("\n  %s%s %s%s\n", Bold, providerIcon, provider, Reset)

		// Models in this group
		for _, m := range providerModels {
			// Skip expired models
			if m.ResetIn == "expired" {
				continue
			}
			bar := ProgressBar(m.RemainingPct, 12)
			color := GetColor(m.RemainingPct)

			// Truncate model name (remove provider prefix for cleaner look)
			displayName := strings.TrimPrefix(m.Name, "Gemini ")
			displayName = strings.TrimPrefix(displayName, "Claude ")
			displayName = strings.TrimPrefix(displayName, "GPT-")
			displayName = Truncate(displayName, 26)

			fmt.Printf("    %-26s %s %s%6.1f%%%s  %s%s%s\n",
				displayName,
				bar,
				color, m.RemainingPct, Reset,
				Dim, m.ResetIn, Reset)
		}
	}

	fmt.Println()

	// Print low quota warnings
	if criticalQuota > 0 || lowQuota > 0 {
		fmt.Println("  " + Dim + strings.Repeat("─", 62) + Reset)
		fmt.Printf("  %s⚠ Quota alerts:%s\n", Yellow, Reset)
		for _, m := range quota.Models {
			// Skip expired models
			if m.ResetIn == "expired" {
				continue
			}
			if m.RemainingPct <= 10 {
				fmt.Printf("    %s●%s %s: %s%.1f%% remaining%s (resets %s)\n",
					Red, Reset, m.Name, Red, m.RemainingPct, Reset, m.ResetIn)
			} else if m.RemainingPct <= 30 {
				fmt.Printf("    %s●%s %s: %s%.1f%% remaining%s (resets %s)\n",
					Yellow, Reset, m.Name, Yellow, m.RemainingPct, Reset, m.ResetIn)
			}
		}
		fmt.Println()
	}
}

// ClearScreen clears the terminal screen
func ClearScreen() {
	fmt.Print("\033[H\033[2J")
}
