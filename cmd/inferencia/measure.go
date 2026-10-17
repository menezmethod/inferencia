package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/menezmethod/inferencia/internal/catalog"
	"github.com/menezmethod/inferencia/internal/ledger"
	"github.com/menezmethod/inferencia/internal/replay"
)

// reportCmd holds the flags measure, savings and subs share: a time window,
// the log sources and, for priced reports, the catalog.
type reportCmd struct {
	fs          *flag.FlagSet
	since       *string
	until       *string
	catalogPath *string
	src         ledger.Sources
	stderr      io.Writer
}

func newReportCmd(name string, withCatalog bool, stderr io.Writer) *reportCmd {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s: warning: no home directory (%v); pass each source flag explicitly\n", name, err)
	}
	def := ledger.DefaultSources(home)
	c := &reportCmd{fs: flag.NewFlagSet(name, flag.ContinueOnError), stderr: stderr}
	c.fs.SetOutput(stderr)
	c.since = c.fs.String("since", "", "start of the window, RFC 3339 or YYYY-MM-DD (inclusive)")
	c.until = c.fs.String("until", "", "end of the window, RFC 3339 or YYYY-MM-DD (inclusive, whole day for a date)")
	c.fs.StringVar(&c.src.ClaudeDir, "claude-dir", def.ClaudeDir, "Claude Code transcripts (empty to skip)")
	c.fs.StringVar(&c.src.CodexDir, "codex-dir", def.CodexDir, "Codex CLI sessions (empty to skip)")
	c.fs.StringVar(&c.src.OpenCodeDB, "opencode-db", def.OpenCodeDB, "OpenCode database (empty to skip)")
	c.fs.StringVar(&c.src.CommandCodeDir, "commandcode-dir", def.CommandCodeDir, "Command Code projects (empty to skip)")
	if withCatalog {
		c.catalogPath = c.fs.String("catalog", "", "price catalog YAML (default: the embedded catalog)")
	}
	return c
}

// load parses args and reads the logs. code is the exit code to return when
// ok is false.
func (c *reportCmd) load(args []string) (us []ledger.Usage, code int, ok bool) {
	name := c.fs.Name()
	if err := c.fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, 0, false
		}
		return nil, 2, false
	}
	if c.fs.NArg() > 0 {
		fmt.Fprintf(c.stderr, "%s: unexpected argument %q\n", name, c.fs.Arg(0))
		return nil, 2, false
	}
	var w ledger.Window
	var err error
	if w.Since, err = parseBound(*c.since, false); err != nil {
		fmt.Fprintf(c.stderr, "%s: --since: %v\n", name, err)
		return nil, 2, false
	}
	if w.Until, err = parseBound(*c.until, true); err != nil {
		fmt.Fprintf(c.stderr, "%s: --until: %v\n", name, err)
		return nil, 2, false
	}
	us, warns, loaded := ledger.Load(c.src, w)
	for _, e := range warns {
		fmt.Fprintf(c.stderr, "%s: warning: %v\n", name, e)
	}
	if loaded == 0 {
		fmt.Fprintf(c.stderr, "%s: no source could be read\n", name)
		return nil, 1, false
	}
	return us, 0, true
}

// loadPriced is load plus the catalog, for the reports that price usage.
func (c *reportCmd) loadPriced(args []string) (us []ledger.Usage, cat *catalog.Catalog, code int, ok bool) {
	if us, code, ok = c.load(args); !ok {
		return nil, nil, code, false
	}
	var err error
	if *c.catalogPath == "" {
		cat, err = catalog.Default()
	} else {
		cat, err = catalog.Load(*c.catalogPath)
	}
	if err != nil {
		fmt.Fprintf(c.stderr, "%s: %v\n", c.fs.Name(), err)
		return nil, nil, 1, false
	}
	return us, cat, 0, true
}

// month is the calendar month the window ends in: the current, partial month
// in a report unless --until says otherwise.
func (c *reportCmd) month() string {
	if t, err := parseBound(*c.until, true); err == nil && !t.IsZero() {
		return t.UTC().Format("2006-01")
	}
	return time.Now().UTC().Format("2006-01")
}

func flush(tw *tabwriter.Writer, name string, stderr io.Writer) bool {
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return false
	}
	return true
}

// runMeasure implements `inferencia measure`: a read-only spend report from
// coding-harness logs. It never writes to the logs and needs no gateway.
func runMeasure(args []string, stdout, stderr io.Writer) int {
	us, code, ok := newReportCmd("measure", false, stderr).load(args)
	if !ok {
		return code
	}
	if err := ledger.WriteReport(stdout, us); err != nil {
		fmt.Fprintf(stderr, "measure: %v\n", err)
		return 1
	}
	return 0
}

// runSavings implements `inferencia savings`: what the logged usage cost at
// list prices, and what it would have cost on other models.
func runSavings(args []string, stdout, stderr io.Writer) int {
	cmd := newReportCmd("savings", true, stderr)
	as := cmd.fs.String("as", "", "comma-separated alternative models (default: the cheapest model per tier on this usage)")
	us, cat, code, ok := cmd.loadPriced(args)
	if !ok {
		return code
	}
	alts := splitList(*as)
	picked := len(alts) == 0
	if picked {
		alts = cat.CheapestPerTier(us)
	}
	lines, unpriced, err := cat.Savings(us, alts)
	if err != nil {
		fmt.Fprintf(stderr, "savings: %v\n", err)
		return 1
	}

	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', tabwriter.AlignRight)
	head := "HARNESS\tUPSTREAM\tMODEL\tDELEGATED\tCALLS\tHARNESS $\tAS RUN $\t"
	for _, a := range alts {
		head += "AS " + a + " $\t"
	}
	fmt.Fprintln(tw, head)
	var total float64
	totalAs := map[string]float64{}
	for _, l := range lines {
		harness := "-"
		if l.Totals.CostCalls > 0 {
			harness = fmt.Sprintf("%.2f", l.Totals.CostUSD)
		}
		row := fmt.Sprintf("%s\t%s\t%s\t%t\t%d\t%s\t%.2f\t", l.Key.Harness, l.Key.Upstream, l.Key.Model, l.Key.Delegated, l.Totals.Calls, harness, l.Actual)
		for _, a := range alts {
			row += fmt.Sprintf("%.2f\t", l.As[a])
			totalAs[a] += l.As[a]
		}
		total += l.Actual
		fmt.Fprintln(tw, row)
	}
	row := fmt.Sprintf("all\t\t\t\t\t\t%.2f\t", total)
	for _, a := range alts {
		lo, hi := totalAs[a]*(1-catalog.TokenizerBand), totalAs[a]*(1+catalog.TokenizerBand)
		row += fmt.Sprintf("%.2f (%.2f-%.2f)\t", totalAs[a], lo, hi)
	}
	fmt.Fprintln(tw, row)
	if !flush(tw, "savings", stderr) {
		return 1
	}
	fmt.Fprintf(stdout, "\nAll dollars are estimates at catalog list prices (as of %s), not bills; plan usage is priced as if paid per token.\n", cat.AsOf)
	fmt.Fprintln(stdout, "HARNESS $ is the harness's own price table; where it differs from AS RUN $, the two price tables disagree.")
	fmt.Fprintf(stdout, "Alternatives keep the same token split; the range is an assumed ±%.0f%% tokenizer difference, not measured.\n", catalog.TokenizerBand*100)
	fmt.Fprintln(stdout, "Anthropic states its newer tokenizer yields about 30% more tokens for the same text, so moving work onto those models can cost more than the range.")
	if picked {
		fmt.Fprintln(stdout, "The alternatives were picked as the cheapest per tier on this same usage, so these are best cases, and quality is not considered.")
	}
	writeUnpriced(stdout, unpriced)
	return 0
}

// runSubs implements `inferencia subs`: the API-equivalent value of the
// tokens each flat plan covered, per month, against each plan tier's price.
func runSubs(args []string, stdout, stderr io.Writer) int {
	cmd := newReportCmd("subs", true, stderr)
	us, cat, code, ok := cmd.loadPriced(args)
	if !ok {
		return code
	}
	pms, unpriced, err := cat.Subs(us)
	if err != nil {
		fmt.Fprintf(stderr, "subs: %v\n", err)
		return 1
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "PLAN\tMONTH\tCALLS\tINPUT\tCACHE READ\tAPI-EQUIVALENT $\tVALUE PER $ PAID, BY TIER\t")
	current := cmd.month()
	for _, pm := range pms {
		month := pm.Month
		if month == current {
			month += " (partial)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%.1f%%\t%.2f\t%s\t\n", pm.Plan.ID, month, pm.Totals.Calls, pm.Totals.AllInput(),
			pm.Totals.CacheShare()*100, pm.Value, tierRatios(pm))
	}
	if !flush(tw, "subs", stderr) {
		return 1
	}
	fmt.Fprintf(stdout, "\nAPI-equivalent $ prices the plan's tokens at catalog list prices (as of %s). A ratio of 5.0x means the plan\n"+
		"delivered five times its price in per-token value. Which tier you hold is not in the logs, so every tier is shown.\n"+
		"claude: Claude Code transcripts do not say whether a call billed to a plan or an API key; read it as \"if on a plan\".\n"+
		"Max is listed only as \"from $100\"; a higher Max tier lowers its ratio. A (partial) month is compared with a full month's price.\n", cat.AsOf)
	writeUnpriced(stdout, unpriced)
	return 0
}

// runReplay implements `inferencia replay`: the recorded sessions re-priced
// under routing policies, with the cost of mid-session model switches.
func runReplay(args []string, stdout, stderr io.Writer) int {
	cmd := newReportCmd("replay", true, stderr)
	strong := cmd.fs.String("strong", "", "strong-tier model for delegated work and the main loop (default: cheapest strong on this usage)")
	fast := cmd.fs.String("fast", "", "fast-tier model for the naive router's small calls (default: cheapest fast on this usage)")
	threshold := cmd.fs.Int64("threshold", 128_000, "naive router: calls with a smaller prompt go to the fast model")
	all, cat, code, ok := cmd.loadPriced(args)
	if !ok {
		return code
	}
	us, excluded := replay.Priceable(cat, all)
	picks := map[string]string{}
	for _, id := range cat.CheapestPerTier(us) {
		m, _ := cat.Lookup(id)
		picks[m.Tier] = id
	}
	if *strong == "" {
		*strong = picks["strong"]
	}
	if *fast == "" {
		*fast = picks["fast"]
	}
	frontier := map[string]bool{}
	for _, m := range cat.Models {
		if m.Tier == "frontier" {
			frontier[m.ID] = true
			for _, a := range m.Aliases {
				frontier[a] = true
			}
		}
	}

	type policy struct {
		name string
		p    replay.Policy
		need []string
	}
	policies := []policy{
		{"as-run", replay.AsRun, nil},
		{"delegated -> " + *strong, replay.Floor(*strong), []string{*strong}},
		{"main loop off frontier -> " + *strong, replay.MainOn(frontier, *strong), []string{*strong}},
		{fmt.Sprintf("naive per call (<%dk -> %s, else %s)", *threshold/1000, *fast, *strong), replay.NaivePerCall(*threshold, *fast, *strong), []string{*fast, *strong}},
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "POLICY\tEST $\tVS AS-RUN\tSWITCHES\tCACHE LOSS $\tUNPRICED CALLS\t")
	// UNPRICED CALLS counts calls a policy moved onto a model with no usable price.
	var base float64
	for _, pol := range policies {
		if slices.Contains(pol.need, "") {
			fmt.Fprintf(tw, "%s\t-\t-\t-\t-\t-\t\n", pol.name)
			continue
		}
		r, err := replay.Run(cat, us, pol.name, pol.p)
		if err != nil {
			fmt.Fprintf(stderr, "replay: %s: %v\n", pol.name, err)
			return 1
		}
		if pol.name == "as-run" {
			base = r.USD
		}
		fmt.Fprintf(tw, "%s\t%.2f\t%s\t%d\t%.2f\t%d\t\n", pol.name, r.USD, ratio(r.USD, base), r.Switches, r.CacheLossUSD, r.Unpriced.Calls)
	}

	// Subscription-first: plan-covered usage costs its plan fee, the rest pays per token.
	pms, _, err := cat.Subs(us)
	if err != nil {
		fmt.Fprintf(stderr, "replay: %v\n", err)
		return 1
	}
	var feeLow, feeHigh float64
	for _, pm := range pms {
		lo, hi := math.Inf(1), 0.0
		for _, usd := range pm.Plan.Tiers {
			lo, hi = min(lo, usd), max(hi, usd)
		}
		feeLow, feeHigh = feeLow+lo, feeHigh+hi
	}
	var rest []ledger.Usage
	for _, u := range us {
		if _, ok := cat.PlanFor(u); !ok {
			rest = append(rest, u)
		}
	}
	r, err := replay.Run(cat, rest, "rest", replay.AsRun)
	if err != nil {
		fmt.Fprintf(stderr, "replay: %v\n", err)
		return 1
	}
	fmt.Fprintf(tw, "subscription-first (plan fees, lowest-highest tier, + per-token rest)\t%.2f-%.2f\t%s-%s\t0\t0.00\t%d\t\n",
		feeLow+r.USD, feeHigh+r.USD, ratio(feeLow+r.USD, base), ratio(feeHigh+r.USD, base), r.Unpriced.Calls)
	if !flush(tw, "replay", stderr) {
		return 1
	}
	fmt.Fprintf(stdout, "\nEstimates at catalog list prices (as of %s); no model was called. Policies keep each call's token split.\n", cat.AsOf)
	fmt.Fprintf(stdout, "Every policy runs on the same %d calls; %d calls that cannot be priced as run are excluded from all of them.\n", len(us), excluded.Calls)
	fmt.Fprintln(stdout, "A switch means a call ran on a different model than the call before it in the same session; it pays a")
	fmt.Fprintln(stdout, "cache write for what the old model had cached (CACHE LOSS $). Subagent sessions are separate, so moving them never switches.")
	fmt.Fprintln(stdout, "Subscription-first assumes every plan-covered call fit inside its plan's quota, so it is a lower bound on cost.")
	fmt.Fprintln(stdout, "Quality is not modeled: a cheaper policy is only worth it if E3 shows its accept rate holds.")
	return 0
}

func ratio(v, base float64) string {
	if base == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2fx", v/base)
}

func tierRatios(pm catalog.PlanMonth) string {
	names := make([]string, 0, len(pm.Plan.Tiers))
	for n := range pm.Plan.Tiers {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int { return cmp.Compare(pm.Plan.Tiers[a], pm.Plan.Tiers[b]) })
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s $%.0f: %.1fx", n, pm.Plan.Tiers[n], pm.Value/pm.Plan.Tiers[n]))
	}
	return strings.Join(parts, ", ")
}

func writeUnpriced(w io.Writer, unpriced catalog.Unpriced) {
	if len(unpriced) == 0 {
		return
	}
	names := make([]string, 0, len(unpriced))
	for n := range unpriced {
		names = append(names, n)
	}
	slices.Sort(names)
	fmt.Fprintln(w, "\nUNPRICED (excluded from the dollars above, never counted as $0):")
	for _, n := range names {
		t := unpriced[n]
		fmt.Fprintf(w, "  %s: %d calls, %d input tokens, %d output tokens\n", n, t.Calls, t.AllInput(), t.Output)
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseBound accepts RFC 3339 or a bare date. A bare date used as an upper
// bound means the end of that day (UTC).
func parseBound(s string, end bool) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("want RFC 3339 or YYYY-MM-DD, got %q", s)
	}
	if end {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return t, nil
}
