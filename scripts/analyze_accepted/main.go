// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type op struct {
	AccountAlias string `json:"accountAlias"`
	AssetCode    string `json:"assetCode"`
	BalanceAfter struct {
		Available string `json:"available"`
	} `json:"balanceAfter"`
}
type tx struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	Operations     []op   `json:"operations"`
}

type aliasKey struct {
	Org, Ledger, Alias, Asset string
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}

	return d
}

func fetchAliasAvailable(transURL, auth, org, ledger, alias, asset string) (float64, error) {
	url := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", strings.TrimRight(transURL, "/"), org, ledger, alias)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("X-Request-Id", fmt.Sprintf("corr-%d", time.Now().UnixNano()))

	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.ReadAll(resp.Body)

		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	var body struct {
		Items []struct {
			AssetCode string `json:"assetCode"`
			Available any    `json:"available"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}

	var cur float64

	for _, it := range body.Items {
		if it.AssetCode != asset {
			continue
		}

		switch v := it.Available.(type) {
		case string:
			_, _ = fmt.Sscan(v, &cur)
		case float64:
			cur = v
		default:
			b, err := json.Marshal(v)
			if err == nil {
				_, _ = fmt.Sscan(string(b), &cur)
			}
		}
	}

	return cur, nil
}

type entry struct {
	kind, id string
	tx       tx
}

func readEntries(path string) ([]entry, error) {
	accFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer accFile.Close()

	var entries []entry

	sc := bufio.NewScanner(accFile)
	for sc.Scan() {
		line := sc.Text()

		parts := strings.SplitN(line, " ", 4)
		if len(parts) < 4 {
			continue
		}

		kind, id, js := parts[1], parts[2], parts[3]

		var t tx

		_ = json.Unmarshal([]byte(js), &t)

		entries = append(entries, entry{kind: kind, id: id, tx: t})
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func countFoundMissing(entries []entry, logStr string) (int, int, []string) {
	var found, missing int

	missIDs := make([]string, 0, 50)

	for _, e := range entries {
		if strings.Contains(logStr, e.id) {
			found++
			continue
		}

		missing++

		if len(missIDs) < 50 {
			missIDs = append(missIDs, fmt.Sprintf("%s:%s", e.kind, e.id))
		}
	}

	return found, missing, missIDs
}

func buildMaxAfter(entries []entry) map[aliasKey]float64 {
	maxAfter := map[aliasKey]float64{}

	for _, e := range entries {
		for _, op := range e.tx.Operations {
			if op.AccountAlias == "" || strings.HasPrefix(op.AccountAlias, "@external/") {
				continue
			}

			var after float64

			_, _ = fmt.Sscan(op.BalanceAfter.Available, &after)

			k := aliasKey{
				Org:    e.tx.OrganizationID,
				Ledger: e.tx.LedgerID,
				Alias:  op.AccountAlias,
				Asset:  op.AssetCode,
			}

			if v, ok := maxAfter[k]; !ok || after > v {
				maxAfter[k] = after
			}
		}
	}

	return maxAfter
}

func compareAliasBalances(maxAfter map[aliasKey]float64, transURL, auth string) ([]string, []string) {
	aliasReports := make([]string, 0, len(maxAfter))
	discrepancies := make([]string, 0, len(maxAfter))

	for k, exp := range maxAfter {
		cur, err := fetchAliasAvailable(transURL, auth, k.Org, k.Ledger, k.Alias, k.Asset)
		if err != nil {
			aliasReports = append(aliasReports, fmt.Sprintf("%s/%s alias=%s asset=%s cur=ERR(%v) exp_min=%.2f", k.Org, k.Ledger, k.Alias, k.Asset, err, exp))
			continue
		}

		aliasReports = append(aliasReports, fmt.Sprintf("%s/%s alias=%s asset=%s cur=%.2f exp_min=%.2f", k.Org, k.Ledger, k.Alias, k.Asset, cur, exp))
		if cur+1e-9 < exp {
			discrepancies = append(discrepancies, fmt.Sprintf("ALERT: alias=%s asset=%s current %.2f < expected_min %.2f", k.Alias, k.Asset, cur, exp))
		}
	}

	return aliasReports, discrepancies
}

func main() {
	acceptedPath := flag.String("accepted", "", "path to accepted sample file")
	logPath := flag.String("log", "", "path to container log file")
	outPath := flag.String("out", "", "path to write correlation summary")
	transURL := flag.String("trans", getenv("TRANSACTION_URL", "http://localhost:3001"), "transaction base URL")
	auth := flag.String("auth", getenv("TEST_AUTH_HEADER", ""), "Authorization header value")

	flag.Parse()

	if *acceptedPath == "" || *logPath == "" || *outPath == "" {
		fmt.Fprintf(os.Stderr, "usage: analyze_accepted -accepted <file> -log <file> -out <file> [-trans URL] [-auth TOKEN]\n")
		os.Exit(2)
	}

	logBytes, err := os.ReadFile(*logPath)
	if err != nil {
		panic(err)
	}

	logStr := string(logBytes)

	entries, err := readEntries(*acceptedPath)
	if err != nil {
		panic(err)
	}

	found, missing, missIDs := countFoundMissing(entries, logStr)

	// Build expected minimums per alias from balanceAfter across accepted operations
	maxAfter := buildMaxAfter(entries)

	// Query current balances and compare
	aliasReports, discrepancies := compareAliasBalances(maxAfter, *transURL, *auth)

	report := fmt.Sprintf(
		"Accepted entries: %d\nFound in logs: %d\nMissing in logs: %d\nSample missing (first %d):\n%s\n\nAlias balance checks (current vs expected_min from balanceAfter):\n%s\n\nDiscrepancies:\n%s\n",
		len(entries), found, missing, len(missIDs), strings.Join(missIDs, "\n"), strings.Join(aliasReports, "\n"), strings.Join(discrepancies, "\n"),
	)

	if err := os.WriteFile(*outPath, []byte(report), 0o644); err != nil {
		panic(err)
	}
}
