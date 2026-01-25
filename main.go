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
	"sync"
	"syscall"
	"time"

	"quota-ag/internal/auth"
	"quota-ag/internal/client"
	"quota-ag/internal/display"
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
	clearScreen := flag.Bool("clear", false, "Clear screen before each refresh (use with --watch)")
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
		runWatchMode(ctx, oauth, *account, *allAccounts, *jsonOutput, *watch, *clearScreen)
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
		accounts, err := oauth.ListAccounts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to list accounts: %v\n", err)
		} else if len(accounts) > 0 {
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

	display.SortModels(quota)

	if jsonOutput {
		printJSON(quota)
	} else {
		display.PrintTable(quota)
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

	// Fetch quotas in parallel for better performance
	type quotaResult struct {
		quota *models.QuotaStatus
		email string
		err   error
	}

	results := make(chan quotaResult, len(tokens))
	var wg sync.WaitGroup

	for _, t := range tokens {
		wg.Add(1)
		go func(token models.AccountCredential) {
			defer wg.Done()
			quota, err := fetchQuotaForToken(ctx, token.AccessToken)
			results <- quotaResult{quota: quota, email: token.Email, err: err}
		}(t)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results with pre-allocated slice
	allQuotas := make([]*models.QuotaStatus, 0, len(tokens))
	for r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch quota for %s: %v\n", r.email, r.err)
			continue
		}
		r.quota.Email = r.email
		display.SortModels(r.quota)
		allQuotas = append(allQuotas, r.quota)
	}

	// Sort by email for consistent output order
	sort.Slice(allQuotas, func(i, j int) bool {
		return allQuotas[i].Email < allQuotas[j].Email
	})

	if jsonOutput {
		printJSONAll(allQuotas)
	} else {
		for i, quota := range allQuotas {
			if i > 0 {
				fmt.Println()
			}
			display.PrintTable(quota)
		}
	}
}

func fetchQuotaForToken(ctx context.Context, accessToken string) (*models.QuotaStatus, error) {
	apiClient := client.NewCloudCodeClient(accessToken)
	return apiClient.FetchQuota(ctx)
}

func printJSON(quota *models.QuotaStatus) {
	data, err := json.MarshalIndent(quota, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func printJSONAll(quotas []*models.QuotaStatus) {
	data, err := json.MarshalIndent(quotas, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
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

func runWatchMode(ctx context.Context, oauth *auth.OAuthClient, account string, allAccounts bool, jsonOutput bool, interval time.Duration, doClear bool) {
	// Setup signal handling for graceful exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial display
	if doClear {
		display.ClearScreen()
	}
	displayQuota(ctx, oauth, account, allAccounts, jsonOutput, interval)

	for {
		select {
		case <-sigChan:
			fmt.Println("\n\nStopping watch mode...")
			return
		case <-ticker.C:
			if doClear {
				display.ClearScreen()
			}
			displayQuota(ctx, oauth, account, allAccounts, jsonOutput, interval)
		}
	}
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
		accounts, err := oauth.ListAccounts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to list accounts: %v\n", err)
		} else if len(accounts) > 0 {
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

	display.SortModels(quota)

	if jsonOutput {
		printJSON(quota)
	} else {
		display.PrintTable(quota)
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

	// Fetch quotas in parallel for better performance
	type quotaResult struct {
		quota *models.QuotaStatus
		email string
		err   error
	}

	results := make(chan quotaResult, len(tokens))
	var wg sync.WaitGroup

	for _, t := range tokens {
		wg.Add(1)
		go func(token models.AccountCredential) {
			defer wg.Done()
			quota, err := fetchQuotaForToken(ctx, token.AccessToken)
			results <- quotaResult{quota: quota, email: token.Email, err: err}
		}(t)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results with pre-allocated slice
	allQuotas := make([]*models.QuotaStatus, 0, len(tokens))
	for r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch quota for %s: %v\n", r.email, r.err)
			continue
		}
		r.quota.Email = r.email
		display.SortModels(r.quota)
		allQuotas = append(allQuotas, r.quota)
	}

	// Sort by email for consistent output order
	sort.Slice(allQuotas, func(i, j int) bool {
		return allQuotas[i].Email < allQuotas[j].Email
	})

	if jsonOutput {
		printJSONAll(allQuotas)
	} else {
		for i, quota := range allQuotas {
			if i > 0 {
				fmt.Println()
			}
			display.PrintTable(quota)
		}
	}
}
