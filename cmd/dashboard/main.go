package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	reset      = "\033[0m"
	bold       = "\033[1m"
	dim        = "\033[2m"
	altScreen  = "\033[?1049h"
	exitAlt    = "\033[?1049l"
	hideCursor = "\033[?25l"
	showCursor = "\033[?25h"
)

type palette struct {
	accent string
	line   string
	glow   string
	muted  string
}

type metricsSnapshot struct {
	InFlight      int64  `json:"in_flight"`
	Rejected      uint64 `json:"rejected"`
	TimedOut      uint64 `json:"timed_out"`
	TotalRequests uint64 `json:"total_requests"`
}

type metricCard struct {
	Title        string
	Value        int64
	Delta        int64
	Max          int64
	Palette      palette
	Data         []int64
	IsCumulative bool
}

type model struct {
	Addr        string
	Interval    time.Duration
	Width       int
	Client      *http.Client
	Last        metricsSnapshot
	LastErr     error
	UpdatedAt   time.Time
	ConnectedAt time.Time
	Histories   map[string][]int64
}

func main() {
	addr := flag.String("addr", "http://localhost:8080/metrics", "metrics endpoint URL")
	interval := flag.Duration("interval", time.Second, "poll interval")
	width := flag.Int("width", 64, "graph width in braille cells")
	flag.Parse()

	if *width < 20 {
		*width = 20
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	m := &model{
		Addr:     *addr,
		Interval: *interval,
		Width:    *width,
		Client:   &http.Client{Timeout: 3 * time.Second},
		Histories: map[string][]int64{
			"total":    {},
			"inflight": {},
			"reject":   {},
			"timeout":  {},
		},
	}

	fmt.Print(altScreen + hideCursor)
	defer fmt.Print(showCursor + exitAlt + reset)

	m.fetch(ctx)
	m.render()

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.fetch(ctx)
			m.render()
		}
	}
}

func (m *model) fetch(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.Addr, nil)
	if err != nil {
		m.LastErr = err
		m.UpdatedAt = time.Now()
		return
	}

	resp, err := m.Client.Do(req)
	if err != nil {
		m.LastErr = err
		m.UpdatedAt = time.Now()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		m.LastErr = fmt.Errorf("unexpected status: %s", resp.Status)
		m.UpdatedAt = time.Now()
		return
	}

	var snap metricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		m.LastErr = err
		m.UpdatedAt = time.Now()
		return
	}

	if m.ConnectedAt.IsZero() {
		m.ConnectedAt = time.Now()
	}

	m.LastErr = nil
	m.Last = snap
	m.UpdatedAt = time.Now()
	m.push("total", int64(snap.TotalRequests))
	m.push("inflight", snap.InFlight)
	m.push("reject", int64(snap.Rejected))
	m.push("timeout", int64(snap.TimedOut))
}

func (m *model) push(key string, v int64) {
	history := append(m.Histories[key], v)
	if len(history) > m.Width {
		history = history[len(history)-m.Width:]
	}
	m.Histories[key] = history
}

func (m *model) render() {
	var b strings.Builder
	b.WriteString("\033[H")

	header := gradient(
		" INFERENCE GATEWAY DASHBOARD ",
		rgb(255, 219, 102),
		rgb(120, 210, 255),
	)
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(dim + " live metrics, braille area charts, rolling history " + reset + "\n\n")

	statusColor := rgb(116, 235, 154)
	statusText := "CONNECTED"
	if m.LastErr != nil {
		statusColor = rgb(255, 107, 107)
		statusText = "DEGRADED"
	}

	meta := []string{
		colorize("endpoint", rgb(148, 163, 184)) + " " + m.Addr,
		colorize("refresh", rgb(148, 163, 184)) + " " + m.Interval.String(),
		colorize("updated", rgb(148, 163, 184)) + " " + m.UpdatedAt.Format("15:04:05"),
		colorize("status", rgb(148, 163, 184)) + " " + statusColor + bold + statusText + reset,
	}
	b.WriteString(joinMeta(meta))
	b.WriteString("\n\n")

	if m.LastErr != nil {
		b.WriteString(errorBox(m.LastErr.Error()))
		b.WriteString("\n\n")
	}

	cards := []metricCard{
		{
			Title:        "Total Requests",
			Value:        int64(m.Last.TotalRequests),
			Delta:        delta(m.Histories["total"]),
			Max:          maxValue(m.Histories["total"]),
			Palette:      palette{accent: rgb(120, 210, 255), line: rgb(94, 234, 212), glow: bgRGB(15, 23, 42), muted: rgb(191, 219, 254)},
			Data:         m.Histories["total"],
			IsCumulative: true,
		},
		{
			Title:        "In Flight",
			Value:        m.Last.InFlight,
			Delta:        delta(m.Histories["inflight"]),
			Max:          maxValue(m.Histories["inflight"]),
			Palette:      palette{accent: rgb(255, 209, 102), line: rgb(255, 159, 28), glow: bgRGB(36, 23, 0), muted: rgb(253, 230, 138)},
			Data:         m.Histories["inflight"],
			IsCumulative: false,
		},
		{
			Title:        "Rejected",
			Value:        int64(m.Last.Rejected),
			Delta:        delta(m.Histories["reject"]),
			Max:          maxValue(m.Histories["reject"]),
			Palette:      palette{accent: rgb(255, 138, 128), line: rgb(255, 107, 107), glow: bgRGB(43, 17, 25), muted: rgb(254, 202, 202)},
			Data:         m.Histories["reject"],
			IsCumulative: true,
		},
		{
			Title:        "Timed Out",
			Value:        int64(m.Last.TimedOut),
			Delta:        delta(m.Histories["timeout"]),
			Max:          maxValue(m.Histories["timeout"]),
			Palette:      palette{accent: rgb(216, 180, 254), line: rgb(192, 132, 252), glow: bgRGB(28, 18, 48), muted: rgb(233, 213, 255)},
			Data:         m.Histories["timeout"],
			IsCumulative: true,
		},
	}

	leftTop := cardBox(cards[0], m.Width)
	rightTop := cardBox(cards[1], m.Width)
	leftBottom := cardBox(cards[2], m.Width)
	rightBottom := cardBox(cards[3], m.Width)

	b.WriteString(joinColumns(leftTop, rightTop, 4))
	b.WriteString("\n")
	b.WriteString(joinColumns(leftBottom, rightBottom, 4))
	b.WriteString("\n\n")

	b.WriteString(footer(m.ConnectedAt, m.Interval, m.Width))

	fmt.Print(b.String())
}

func cardBox(card metricCard, width int) string {
	plot := braillePlot(card.Data, width, 4)
	innerWidth := maxLineWidth(plot)
	if innerWidth < 52 {
		innerWidth = 52
	}

	title := chip(card.Title, card.Palette.accent, card.Palette.glow) + " " + colorize(metricMode(card.IsCumulative), card.Palette.muted)
	value := bold + colorize(formatNumber(card.Value), card.Palette.line) + reset
	deltaText := signedDelta(card.Delta)
	deltaColor := rgb(148, 163, 184)
	if card.Delta > 0 {
		deltaColor = card.Palette.line
	} else if card.Delta < 0 {
		deltaColor = rgb(255, 179, 71)
	}

	stats := joinMeta([]string{
		colorize("now", card.Palette.muted) + " " + value,
		colorize("delta", card.Palette.muted) + " " + colorize(deltaText, deltaColor),
		colorize("peak", card.Palette.muted) + " " + colorize(formatNumber(card.Max), card.Palette.line),
	})

	lines := []string{
		title,
		stats,
		dim + " history " + strings.Repeat("•", 3) + " newest sample at right " + reset,
	}
	lines = append(lines, splitAndPadLines(colorize(plot, card.Palette.line), innerWidth)...)

	border := colorize("─", card.Palette.accent)
	top := colorize("╭", card.Palette.accent) + strings.Repeat(border, innerWidth+2) + colorize("╮", card.Palette.accent)
	bottom := colorize("╰", card.Palette.accent) + strings.Repeat(border, innerWidth+2) + colorize("╯", card.Palette.accent)

	var b strings.Builder
	b.WriteString(top)
	b.WriteByte('\n')
	for _, line := range lines {
		b.WriteString(colorize("│", card.Palette.accent))
		b.WriteByte(' ')
		b.WriteString(padVisible(line, innerWidth))
		b.WriteByte(' ')
		b.WriteString(colorize("│", card.Palette.accent))
		b.WriteByte('\n')
	}
	b.WriteString(bottom)
	return b.String()
}

func braillePlot(values []int64, width, heightCells int) string {
	if len(values) == 0 {
		return dim + "waiting for samples" + reset
	}

	dotWidth := maxInt(width*2, 2)
	dotHeight := maxInt(heightCells*4, 4)
	samples := resample(values, dotWidth)

	minV := minValue(samples)
	maxV := maxValue(samples)
	if maxV == minV {
		maxV = minV + 1
	}

	grid := make([][]bool, dotHeight)
	for y := range grid {
		grid[y] = make([]bool, dotWidth)
	}

	yPositions := make([]int, len(samples))
	var prevX, prevY int
	for x, v := range samples {
		normalized := float64(v-minV) / float64(maxV-minV)
		y := dotHeight - 1 - int(math.Round(normalized*float64(dotHeight-1)))
		yPositions[x] = y
		if x == 0 {
			grid[y][x] = true
			prevX, prevY = x, y
			continue
		}
		drawLine(grid, prevX, prevY, x, y)
		prevX, prevY = x, y
	}

	for x, y := range yPositions {
		for fillY := y; fillY < dotHeight; fillY++ {
			grid[fillY][x] = true
		}
	}

	var out strings.Builder
	topLabel := formatCompact(maxV)
	bottomLabel := formatCompact(minV)
	for row := 0; row < dotHeight; row += 4 {
		if row == 0 {
			out.WriteString(padLeft(topLabel, 5))
		} else if row+4 >= dotHeight {
			out.WriteString(padLeft(bottomLabel, 5))
		} else {
			out.WriteString("     ")
		}
		out.WriteString(" │")
		for col := 0; col < dotWidth; col += 2 {
			out.WriteRune(brailleCell(grid, row, col))
		}
		out.WriteString("│")
		if row+4 < dotHeight {
			out.WriteByte('\n')
		}
	}
	out.WriteByte('\n')
	out.WriteString("      ")
	out.WriteString(strings.Repeat("⠉", width))
	out.WriteString(" ")
	out.WriteString(dim + "now" + reset)
	return out.String()
}

func brailleCell(grid [][]bool, row, col int) rune {
	var mask rune
	dots := []struct {
		dx  int
		dy  int
		bit rune
	}{
		{0, 0, 1},
		{0, 1, 2},
		{0, 2, 4},
		{1, 0, 8},
		{1, 1, 16},
		{1, 2, 32},
		{0, 3, 64},
		{1, 3, 128},
	}

	for _, dot := range dots {
		r := row + dot.dy
		c := col + dot.dx
		if r < len(grid) && c < len(grid[r]) && grid[r][c] {
			mask |= dot.bit
		}
	}

	if mask == 0 {
		return ' '
	}
	return rune(0x2800 + mask)
}

func drawLine(grid [][]bool, x0, y0, x1, y1 int) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy

	for {
		if y0 >= 0 && y0 < len(grid) && x0 >= 0 && x0 < len(grid[y0]) {
			grid[y0][x0] = true
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func resample(values []int64, width int) []int64 {
	if len(values) == width {
		return append([]int64(nil), values...)
	}

	result := make([]int64, width)
	if len(values) == 1 {
		for i := range result {
			result[i] = values[0]
		}
		return result
	}

	for i := 0; i < width; i++ {
		pos := float64(i) * float64(len(values)-1) / float64(width-1)
		left := int(math.Floor(pos))
		right := int(math.Ceil(pos))
		if left == right {
			result[i] = values[left]
			continue
		}
		ratio := pos - float64(left)
		result[i] = int64(math.Round(float64(values[left])*(1-ratio) + float64(values[right])*ratio))
	}
	return result
}

func footer(connectedAt time.Time, interval time.Duration, width int) string {
	age := "just started"
	if !connectedAt.IsZero() {
		age = time.Since(connectedAt).Round(time.Second).String()
	}

	parts := []string{
		colorize("q", rgb(255, 219, 102)) + dim + " ctrl+c to exit" + reset,
		colorize("history", rgb(120, 210, 255)) + dim + " " + strconv.Itoa(intervalHistory(interval, width)) + "s visible" + reset,
		colorize("uptime", rgb(116, 235, 154)) + dim + " " + age + reset,
	}
	return joinMeta(parts)
}

func intervalHistory(interval time.Duration, width int) int {
	return int((time.Duration(width) * interval) / time.Second)
}

func joinColumns(left, right string, gap int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	width := maxStringLen(leftLines)
	rows := maxInt(len(leftLines), len(rightLines))

	var b strings.Builder
	for i := 0; i < rows; i++ {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		b.WriteString(padVisible(l, width))
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(r)
		if i+1 < rows {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func errorBox(msg string) string {
	width := visibleLen(msg)
	if width < 36 {
		width = 36
	}
	top := colorize("╭"+strings.Repeat("─", width+2)+"╮", rgb(255, 107, 107))
	body := colorize("│", rgb(255, 107, 107)) + " " + padVisible(colorize("error: "+msg, rgb(255, 138, 128)), width) + " " + colorize("│", rgb(255, 107, 107))
	bottom := colorize("╰"+strings.Repeat("─", width+2)+"╯", rgb(255, 107, 107))
	return top + "\n" + body + "\n" + bottom
}

func joinMeta(parts []string) string {
	return strings.Join(parts, dim+"  │  "+reset)
}

func padLine(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = padVisible(line, width)
	}
	return strings.Join(lines, "\n")
}

func splitAndPadLines(s string, width int) []string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = padVisible(line, width)
	}
	return lines
}

func padVisible(s string, width int) string {
	padding := width - visibleLen(s)
	if padding <= 0 {
		return s
	}
	return s + strings.Repeat(" ", padding)
}

func visibleLen(s string) int {
	length := 0
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\033':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			length++
		}
	}
	return length
}

func maxLineWidth(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if l := visibleLen(line); l > max {
			max = l
		}
	}
	return max
}

func maxStringLen(lines []string) int {
	max := 0
	for _, line := range lines {
		if l := visibleLen(line); l > max {
			max = l
		}
	}
	return max
}

func gradient(text, left, right string) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}

	lr, lg, lb := parseRGB(left)
	rr, rg, rb := parseRGB(right)
	var b strings.Builder
	for i, r := range runes {
		t := float64(i) / float64(maxInt(len(runes)-1, 1))
		cr := int(math.Round(float64(lr)*(1-t) + float64(rr)*t))
		cg := int(math.Round(float64(lg)*(1-t) + float64(rg)*t))
		cb := int(math.Round(float64(lb)*(1-t) + float64(rb)*t))
		b.WriteString(rgb(cr, cg, cb))
		if i == 0 {
			b.WriteString(bold)
		}
		b.WriteRune(r)
	}
	b.WriteString(reset)
	return b.String()
}

func parseRGB(code string) (int, int, int) {
	var r, g, b int
	fmt.Sscanf(code, "\033[38;2;%d;%d;%dm", &r, &g, &b)
	return r, g, b
}

func rgb(r, g, b int) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

func bgRGB(r, g, b int) string {
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b)
}

func colorize(text, color string) string {
	return color + text + reset
}

func chip(text, fg, bg string) string {
	return bg + fg + bold + " " + text + " " + reset
}

func metricMode(cumulative bool) string {
	if cumulative {
		return "cumulative"
	}
	return "live gauge"
}

func formatNumber(v int64) string {
	s := strconv.FormatInt(v, 10)
	if len(s) <= 3 {
		return s
	}

	var out []byte
	rem := len(s) % 3
	if rem > 0 {
		out = append(out, s[:rem]...)
		if len(s) > rem {
			out = append(out, ',')
		}
	}
	for i := rem; i < len(s); i += 3 {
		out = append(out, s[i:i+3]...)
		if i+3 < len(s) {
			out = append(out, ',')
		}
	}
	return string(out)
}

func formatCompact(v int64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(v)/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.1fk", float64(v)/1_000)
	default:
		return strconv.FormatInt(v, 10)
	}
}

func signedDelta(v int64) string {
	if v > 0 {
		return "+" + formatNumber(v) + " this tick"
	}
	if v < 0 {
		return "-" + formatNumber(abs64(v)) + " this tick"
	}
	return "flat this tick"
}

func delta(values []int64) int64 {
	if len(values) < 2 {
		return 0
	}
	return values[len(values)-1] - values[len(values)-2]
}

func maxValue(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func minValue(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
