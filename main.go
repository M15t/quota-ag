package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"quota-ag/internal/auth"
	"quota-ag/internal/client"
	"quota-ag/internal/models"
)

func main() {
	// Check for --remove without argument before flag.Parse()
	for i, arg := range os.Args[1:] {
		if arg == "--remove" || arg == "-remove" {
			// Check if next arg exists and is not another flag
			if i+2 >= len(os.Args) || strings.HasPrefix(os.Args[i+2], "-") {
				showRemoveHelp()
				return
			}
		}
	}

	// Parse flags
	jsonOutput := flag.Bool("json", false, "Output in JSON format")
	addAccount := flag.Bool("add", false, "Add a new account")
	listAccounts := flag.Bool("list", false, "List all stored accounts")
	removeAccount := flag.String("remove", "", "Remove an account by email")
	account := flag.String("account", "", "Use specific account (email)")
	allAccounts := flag.Bool("all", false, "Show quota for all accounts")
	watch := flag.Duration("watch", 0, "Auto-refresh interval (e.g., 30s, 1m, 5m)")
	flag.Parse()

	ctx := context.Background()
	oauth := auth.NewOAuthClient()

	// Handle account management commands
	if *listAccounts {
		accounts, err := oauth.ListAccounts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(accounts) == 0 {
			fmt.Println("No accounts stored. Use --add to add an account.")
		} else {
			fmt.Println("Stored accounts:")
			for _, email := range accounts {
				fmt.Printf("  • %s\n", email)
			}
		}
		return
	}

	if *removeAccount != "" {
		if err := oauth.RemoveAccount(*removeAccount); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed account: %s\n", *removeAccount)
		return
	}

	if *addAccount {
		accessToken, userInfo, err := oauth.GetAccessToken(ctx, "", true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		_ = accessToken
		fmt.Printf("Added account: %s\n", userInfo.Email)
		return
	}

	// Fetch and display quota
	if *watch > 0 {
		// Watch mode with auto-refresh
		runWatchMode(ctx, oauth, *account, *allAccounts, *jsonOutput, *watch)
	} else if *allAccounts {
		// Show quota for all accounts
		showAllAccountsQuota(ctx, oauth, *jsonOutput)
	} else {
		// Show quota for single account
		showSingleAccountQuota(ctx, oauth, *account, *jsonOutput)
	}
}

func showSingleAccountQuota(ctx context.Context, oauth *auth.OAuthClient, email string, jsonOutput bool) {
	// If no account specified, try to use first available or prompt for auth
	if email == "" {
		accounts, _ := oauth.ListAccounts()
		if len(accounts) > 0 {
			email = accounts[0]
		}
	}

	accessToken, userInfo, err := oauth.GetAccessToken(ctx, email, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	quota, err := fetchQuotaForToken(ctx, accessToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if userInfo != nil {
		quota.Email = userInfo.Email
	}

	sortModels(quota)

	if jsonOutput {
		printJSON(quota)
	} else {
		printTable(quota)
	}
}

func showAllAccountsQuota(ctx context.Context, oauth *auth.OAuthClient, jsonOutput bool) {
	tokens, err := oauth.GetAllAccountTokens(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(tokens) == 0 {
		fmt.Println("No accounts stored. Use --add to add an account.")
		return
	}

	var allQuotas []*models.QuotaStatus

	for _, t := range tokens {
		quota, err := fetchQuotaForToken(ctx, t.AccessToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch quota for %s: %v\n", t.Email, err)
			continue
		}
		quota.Email = t.Email
		sortModels(quota)
		allQuotas = append(allQuotas, quota)
	}

	if jsonOutput {
		printJSONAll(allQuotas)
	} else {
		for i, quota := range allQuotas {
			if i > 0 {
				fmt.Println()
			}
			printTable(quota)
		}
	}
}

func fetchQuotaForToken(ctx context.Context, accessToken string) (*models.QuotaStatus, error) {
	apiClient := client.NewCloudCodeClient(accessToken)
	return apiClient.FetchQuota(ctx)
}

func sortModels(quota *models.QuotaStatus) {
	sort.Slice(quota.Models, func(i, j int) bool {
		return quota.Models[i].Name < quota.Models[j].Name
	})
}

func printJSON(quota *models.QuotaStatus) {
	data, _ := json.MarshalIndent(quota, "", "  ")
	fmt.Println(string(data))
}

func printJSONAll(quotas []*models.QuotaStatus) {
	data, _ := json.MarshalIndent(quotas, "", "  ")
	fmt.Println(string(data))
}

func printTable(quota *models.QuotaStatus) {
	if len(quota.Models) == 0 {
		fmt.Println("No models found.")
		return
	}

	// Header
	fmt.Println()
	fmt.Println("  Antigravity Quota Status")
	if quota.Email != "" {
		fmt.Printf("  Account: \033[36m%s\033[0m\n", quota.Email[:5]+"..."+strings.Split(quota.Email, "@")[1])
	}
	fmt.Println("  " + strings.Repeat("━", 60))
	fmt.Printf("  %-30s %17s %10s\n", "Model", "Remaining", "Reset In")
	fmt.Println("  " + strings.Repeat("─", 60))

	// Rows
	for _, m := range quota.Models {
		if m.Name == "" {
			continue
		}
		bar := progressBar(m.RemainingPct, 10)
		color := getColor(m.RemainingPct)
		reset := "\033[0m"

		fmt.Printf("  %-30s %s%s %5.1f%%%s %10s\n",
			truncate(m.Name, 30),
			color, bar, m.RemainingPct, reset,
			m.ResetIn,
		)
	}

	fmt.Println("  " + strings.Repeat("━", 60))
}

func progressBar(pct float64, width int) string {
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

func getColor(pct float64) string {
	if pct <= 10 {
		return "\033[31m" // Red
	} else if pct <= 30 {
		return "\033[33m" // Yellow
	}
	return "\033[32m" // Green
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func showRemoveHelp() {
	oauth := auth.NewOAuthClient()
	accounts, err := oauth.ListAccounts()
	if err != nil || len(accounts) == 0 {
		fmt.Println("No accounts stored. Use --add to add an account first.")
		return
	}

	fmt.Println("Select an account to remove:")
	fmt.Println()
	for i, email := range accounts {
		fmt.Printf("  [%d] %s\n", i+1, email)
	}
	fmt.Println("  [0] Cancel")
	fmt.Println()
	fmt.Print("Enter number: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		return
	}

	input = strings.TrimSpace(input)
	choice, err := strconv.Atoi(input)
	if err != nil || choice < 0 || choice > len(accounts) {
		fmt.Println("Invalid selection.")
		return
	}

	if choice == 0 {
		fmt.Println("Cancelled.")
		return
	}

	email := accounts[choice-1]
	if err := oauth.RemoveAccount(email); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	fmt.Printf("Removed account: %s\n", email)
}

func runWatchMode(ctx context.Context, oauth *auth.OAuthClient, account string, allAccounts bool, jsonOutput bool, interval time.Duration) {
	// Setup signal handling for graceful exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial display
	clearScreen()
	displayQuota(ctx, oauth, account, allAccounts, jsonOutput, interval)

	for {
		select {
		case <-sigChan:
			fmt.Println("\n\nStopping watch mode...")
			return
		case <-ticker.C:
			clearScreen()
			displayQuota(ctx, oauth, account, allAccounts, jsonOutput, interval)
		}
	}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func displayQuota(ctx context.Context, oauth *auth.OAuthClient, account string, allAccounts bool, jsonOutput bool, interval time.Duration) {
	// Show refresh info
	if !jsonOutput {
		fmt.Printf("  \033[90mAuto-refreshing every %s (Ctrl+C to stop)\033[0m\n", interval)
		fmt.Printf("  \033[90mLast updated: %s\033[0m\n", time.Now().Format("15:04:05"))
	}

	if allAccounts {
		showAllAccountsQuotaSilent(ctx, oauth, jsonOutput)
	} else {
		showSingleAccountQuotaSilent(ctx, oauth, account, jsonOutput)
	}
}

func showSingleAccountQuotaSilent(ctx context.Context, oauth *auth.OAuthClient, email string, jsonOutput bool) {
	if email == "" {
		accounts, _ := oauth.ListAccounts()
		if len(accounts) > 0 {
			email = accounts[0]
		}
	}

	accessToken, userInfo, err := oauth.GetAccessToken(ctx, email, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	quota, err := fetchQuotaForToken(ctx, accessToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	if userInfo != nil {
		quota.Email = userInfo.Email
	}

	sortModels(quota)

	if jsonOutput {
		printJSON(quota)
	} else {
		printTable(quota)
	}
}

func showAllAccountsQuotaSilent(ctx context.Context, oauth *auth.OAuthClient, jsonOutput bool) {
	tokens, err := oauth.GetAllAccountTokens(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	if len(tokens) == 0 {
		fmt.Println("No accounts stored. Use --add to add an account.")
		return
	}

	var allQuotas []*models.QuotaStatus

	for _, t := range tokens {
		quota, err := fetchQuotaForToken(ctx, t.AccessToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch quota for %s: %v\n", t.Email, err)
			continue
		}
		quota.Email = t.Email
		sortModels(quota)
		allQuotas = append(allQuotas, quota)
	}

	if jsonOutput {
		printJSONAll(allQuotas)
	} else {
		for i, quota := range allQuotas {
			if i > 0 {
				fmt.Println()
			}
			printTable(quota)
		}
	}
}
