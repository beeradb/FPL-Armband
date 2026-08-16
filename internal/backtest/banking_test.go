package backtest

// Does the policy ever bank transfers, and can it reach a premium if it does?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBanking -v -timeout 30m
//
// A premium upgrade usually needs more than one move: the money is locked in a
// player of a different position, so buying him means selling a forward *and*
// funding the difference from somewhere else. RankPairs can express that — one
// upgrade against several sales — but only if there are transfers in hand to
// spend, and the weekly decision is greedy: it spends a free transfer the moment
// any move clears the gate.
//
// So the question is not whether a three-way swap is *expressible*. It is
// whether the policy ever accumulates the transfers such a swap would need.

import (
	"fmt"
	"os"
	"testing"
)

func TestDiagBanking(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	pairs := sweepPairNames()

	freeHist := map[int]int{}
	movesHist := map[int]int{}
	weeks, totalMoves := 0, 0

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		sc := sweepConfig(cfg, 1, true)
		res, err := Simulate(cur, prior, sc)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range res.Weeks {
			freeHist[w.Free]++
			weeks++
		}
		byGW := map[int]int{}
		for _, mv := range res.Moves {
			byGW[mv.GW]++
		}
		for _, n := range byGW {
			movesHist[n]++
			totalMoves += n
		}
	}

	fmt.Printf("\nFree transfers in hand when each weekly decision was made,\n")
	fmt.Printf("%s from GW1, bank limit %d:\n\n", seasonsLabel(len(pairs)), sweepBankLimit)
	fmt.Printf("%-14s %8s %8s\n", "in hand", "weeks", "share")
	for f := 0; f <= sweepBankLimit; f++ {
		if freeHist[f] == 0 {
			continue
		}
		fmt.Printf("%-14d %8d %7.0f%%\n", f, freeHist[f],
			100*float64(freeHist[f])/float64(weeks))
	}

	fmt.Printf("\nMoves made in one gameweek, when any were made:\n\n")
	fmt.Printf("%-14s %8s %8s\n", "moves", "weeks", "share")
	var multi int
	for n := 1; n <= 6; n++ {
		if movesHist[n] == 0 {
			continue
		}
		var tot int
		for k := 1; k <= 6; k++ {
			tot += movesHist[k]
		}
		if n >= 3 {
			multi += movesHist[n]
		}
		fmt.Printf("%-14d %8d %7.0f%%\n", n, movesHist[n],
			100*float64(movesHist[n])/float64(tot))
	}
	fmt.Printf("\nweeks with three or more moves — the shape a premium upgrade needs: %d\n", multi)
	fmt.Printf("\nIf the bank is almost always 1, the policy is spending every transfer as\n")
	fmt.Printf("soon as any move clears the gate, and a three-way swap can never be\n")
	fmt.Printf("assembled however well RankPairs could express one.\n")
}
