// sdl2-life
// by Brian C. Lane <bcl@brianlane.com>
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

const (
	threshold = 0.15
	// LinearGradient cmdline selection
	LinearGradient = 0
	// PolylinearGradient cmdline selection
	PolylinearGradient = 1
	// BezierGradient cmdline selection
	BezierGradient = 2
)

// RLE header with variable spacing and optional rules
// Matches it with or without rule at the end, and with 0 or more spaces between elements.
var rleHeaderRegex = regexp.MustCompile(`x\s*=\s*(\d+)\s*,\s*y\s*=\s*(\d+)(?:\s*,\s*rule\s*=\s*(.*))*`)

/* commandline flags */
type cmdlineArgs struct {
	Width       int    // Width of window in pixels
	Height      int    // Height of window in pixels
	CellSize    int    // Cell size in pixels (square)
	Seed        int64  // Seed for PRNG
	Border      bool   // Border around cells
	Font        string // Path to TTF to use for status bar
	FontSize    int    // Size of font in points
	Rule        string // Rulestring to use
	Fps         int    // Frames per Second
	PatternFile string // File with initial pattern
	Pause       bool   // Start the game paused
	Empty       bool   // Start with empty world
	Color       bool   // Color the cells based on age
	Colors      string // Comma separated color hex triplets
	Gradient    int    // Gradient algorithm to use
	MaxAge      int    // Maximum age for gradient colors
	Port        int    // Port to listen to
	Host        string // Host IP to bind to
	Server      bool   // Launch an API server when true
	Rotate      int    // Screen rotation: 0, 90, 180, 270
	StatusTop   bool   // Place status text at the top instead of bottom
}

/* commandline defaults */
var cfg = cmdlineArgs{
	Width:       500,
	Height:      500,
	CellSize:    5,
	Seed:        0,
	Border:      false,
	Font:        "",
	FontSize:    14,
	Rule:        "B3/S23",
	Fps:         10,
	PatternFile: "",
	Pause:       false,
	Empty:       false,
	Color:       false,
	Colors:      "#4682b4,#ffffff",
	Gradient:    0,
	MaxAge:      255,
	Port:        3051,
	Host:        "127.0.0.1",
	Server:      false,
	Rotate:      0,
	StatusTop:   false,
}

/* parseArgs handles parsing the cmdline args and setting values in the global cfg struct */
func parseArgs() {
	flag.IntVar(&cfg.Width, "width", cfg.Width, "Width of window in pixels")
	flag.IntVar(&cfg.Height, "height", cfg.Height, "Height of window in pixels")
	flag.IntVar(&cfg.CellSize, "cell", cfg.CellSize, "Cell size in pixels (square)")
	flag.Int64Var(&cfg.Seed, "seed", cfg.Seed, "PRNG seed")
	flag.BoolVar(&cfg.Border, "border", cfg.Border, "Border around cells")
	flag.StringVar(&cfg.Font, "font", cfg.Font, "Path to TTF to use for status bar")
	flag.IntVar(&cfg.FontSize, "font-size", cfg.FontSize, "Size of font in points")
	flag.StringVar(&cfg.Rule, "rule", cfg.Rule, "Rulestring Bn.../Sn... (B3/S23)")
	flag.IntVar(&cfg.Fps, "fps", cfg.Fps, "Frames per Second update rate (10fps)")
	flag.StringVar(&cfg.PatternFile, "pattern", cfg.PatternFile, "File with initial pattern to load")
	flag.BoolVar(&cfg.Pause, "pause", cfg.Pause, "Start the game paused")
	flag.BoolVar(&cfg.Empty, "empty", cfg.Empty, "Start with empty world")
	flag.BoolVar(&cfg.Color, "color", cfg.Color, "Color cells based on age")
	flag.StringVar(&cfg.Colors, "colors", cfg.Colors, "Comma separated color hex triplets")
	flag.IntVar(&cfg.Gradient, "gradient", cfg.Gradient, "Gradient type. 0=Linear 1=Polylinear 2=Bezier")
	flag.IntVar(&cfg.MaxAge, "age", cfg.MaxAge, "Maximum age for gradient colors")
	flag.IntVar(&cfg.Port, "port", cfg.Port, "Port to listen to")
	flag.StringVar(&cfg.Host, "host", cfg.Host, "Host IP to bind to")
	flag.BoolVar(&cfg.Server, "server", cfg.Server, "Launch an API server")
	flag.IntVar(&cfg.Rotate, "rotate", cfg.Rotate, "Rotate screen by 0°, 90°, 180°, or 270°")
	flag.BoolVar(&cfg.StatusTop, "status-top", cfg.StatusTop, "Status text at the top")

	flag.Parse()

	if cfg.Rotate != 0 && cfg.Rotate != 90 && cfg.Rotate != 180 && cfg.Rotate != 270 {
		log.Fatal("-rotate only supports 0, 90, 180, and 270")
	}
}

// Possible default fonts to search for
var defaultFonts = []string{"/usr/share/fonts/liberation/LiberationMono-Regular.ttf",
	"/usr/local/share/fonts/TerminusTTF/TerminusTTF-4.49.2.ttf",
	"/usr/X11/share/fonts/TTF/LiberationMono-Regular.ttf"}

// RGBAColor holds a color
type RGBAColor struct {
	r, g, b, a uint8
}

// Gradient holds the colors to use for displaying cell age
type Gradient struct {
	controls []RGBAColor
	points   []RGBAColor
}

// Print prints the gradient values to the console
func (g *Gradient) Print() {
	fmt.Printf("controls:\n%#v\n\n", g.controls)
	for i := range g.points {
		fmt.Printf("%d = %#v\n", i, g.points[i])
	}
}

// Append adds the points from a gradient to this one at an insertion point
func (g *Gradient) Append(from Gradient, start int) {
	for i, p := range from.points {
		g.points[start+i] = p
	}
}

// NewLinearGradient returns a Linear Gradient with pre-computed colors for every age
// from https://bsouthga.dev/posts/color-gradients-with-python
//
// Only uses the first and last color passed in
func NewLinearGradient(colors []RGBAColor, maxAge int) (Gradient, error) {
	if len(colors) < 1 {
		return Gradient{}, fmt.Errorf("Linear Gradient requires at least 1 color")
	}

	// Use the first and last color in controls as start and end
	gradient := Gradient{controls: []RGBAColor{colors[0], colors[len(colors)-1]}, points: make([]RGBAColor, maxAge)}

	start := gradient.controls[0]
	end := gradient.controls[1]

	for t := 0; t < maxAge; t++ {
		r := uint8(float64(start.r) + (float64(t)/float64(maxAge-1))*(float64(end.r)-float64(start.r)))
		g := uint8(float64(start.g) + (float64(t)/float64(maxAge-1))*(float64(end.g)-float64(start.g)))
		b := uint8(float64(start.b) + (float64(t)/float64(maxAge-1))*(float64(end.b)-float64(start.b)))
		gradient.points[t] = RGBAColor{r, g, b, 255}
	}
	return gradient, nil
}

// NewPolylinearGradient returns a gradient that is linear between all control colors
func NewPolylinearGradient(colors []RGBAColor, maxAge int) (Gradient, error) {
	if len(colors) < 2 {
		return Gradient{}, fmt.Errorf("Polylinear Gradient requires at least 2 colors")
	}

	gradient := Gradient{controls: colors, points: make([]RGBAColor, maxAge)}

	n := int(float64(maxAge) / float64(len(colors)-1))
	g, _ := NewLinearGradient(colors, n)
	gradient.Append(g, 0)

	if len(colors) == 2 {
		return gradient, nil
	}

	for i := 1; i < len(colors)-1; i++ {
		if i == len(colors)-2 {
			// The last group may need to be extended if it doesn't fill all the way to the end
			remainder := maxAge - ((i + 1) * n)
			g, _ := NewLinearGradient(colors[i:i+1], n+remainder)
			gradient.Append(g, (i * n))
		} else {
			g, _ := NewLinearGradient(colors[i:i+1], n)
			gradient.Append(g, (i * n))
		}
	}

	return gradient, nil
}

// FactorialCache saves the results for factorial calculations for faster access
type FactorialCache struct {
	cache map[int]float64
}

// NewFactorialCache returns a new empty cache
func NewFactorialCache() *FactorialCache {
	return &FactorialCache{cache: make(map[int]float64)}
}

// Fact calculates the n! and caches the results
func (fc *FactorialCache) Fact(n int) float64 {
	f, ok := fc.cache[n]
	if ok {
		return f
	}
	var result float64
	if n == 1 || n == 0 {
		result = 1
	} else {
		result = float64(n) * fc.Fact(n-1)
	}

	fc.cache[n] = result
	return result
}

// Bernstein calculates the bernstein coefficient
//
// t runs from 0 -> 1 and is the 'position' on the curve (age / maxAge-1)
// n is the number of control colors -1
// i is the current control color from 0 -> n
func (fc *FactorialCache) Bernstein(t float64, n, i int) float64 {
	b := fc.Fact(n) / (fc.Fact(i) * fc.Fact(n-i))
	b = b * math.Pow(1-t, float64(n-i)) * math.Pow(t, float64(i))
	return b
}

// NewBezierGradient returns pre-computed colors using control colors and bezier curve
// from https://bsouthga.dev/posts/color-gradients-with-python
func NewBezierGradient(controls []RGBAColor, maxAge int) Gradient {
	gradient := Gradient{controls: controls, points: make([]RGBAColor, maxAge)}
	fc := NewFactorialCache()

	for t := 0; t < maxAge; t++ {
		color := RGBAColor{}
		for i, c := range controls {
			color.r += uint8(fc.Bernstein(float64(t)/float64(maxAge-1), len(controls)-1, i) * float64(c.r))
			color.g += uint8(fc.Bernstein(float64(t)/float64(maxAge-1), len(controls)-1, i) * float64(c.g))
			color.b += uint8(fc.Bernstein(float64(t)/float64(maxAge-1), len(controls)-1, i) * float64(c.b))
		}
		color.a = 255
		gradient.points[t] = color
	}

	return gradient
}

// Cell describes the location and state of a cell
type Cell struct {
	alive     bool
	aliveNext bool

	x int
	y int

	age int
}

// Pattern is used to pass patterns from the API to the game
type Pattern []string

// LifeGame holds all the global state of the game and the methods to operate on it
type LifeGame struct {
	mp        bool
	erase     bool
	cells     [][]*Cell // NOTE: This is an array of [row][columns] not x,y coordinates
	liveCells int
	age       int64
	birth     map[int]bool
	stayAlive map[int]bool

	// Graphics
	window   *sdl.Window
	renderer *sdl.Renderer
	font     *ttf.Font
	bg       RGBAColor
	fg       RGBAColor
	rows     int
	columns  int
	gradient Gradient
	pChan    <-chan Pattern
}

// cleanup will handle cleanup of allocated resources
func (g *LifeGame) cleanup() {
	// Clean up all the allocated memory

	g.renderer.Destroy()
	g.window.Destroy()
	g.font.Close()
	ttf.Quit()
	sdl.Quit()
}

// InitializeCells resets the world, either randomly or from a pattern file
func (g *LifeGame) InitializeCells() {
	g.age = 0

	// Fill it with dead cells first
	g.cells = make([][]*Cell, g.rows, g.columns)
	for y := 0; y < g.rows; y++ {
		for x := 0; x < g.columns; x++ {
			c := &Cell{x: x, y: y}
			g.cells[y] = append(g.cells[y], c)
		}
	}

	if len(cfg.PatternFile) > 0 {
		// Read all of the pattern file for parsing
		f, err := os.Open(cfg.PatternFile)
		if err != nil {
			log.Fatalf("Error reading pattern file: %s", err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if len(lines) == 0 {
			log.Fatalf("%s is empty.", cfg.PatternFile)
		}

		if strings.HasPrefix(lines[0], "#Life 1.05") {
			err = g.ParseLife105(lines)
		} else if strings.HasPrefix(lines[0], "#Life 1.06") {
			log.Fatal("Life 1.06 file format is not supported")
		} else if isRLE(lines) {
			err = g.ParseRLE(lines, 0, 0)
		} else {
			err = g.ParsePlaintext(lines)
		}

		if err != nil {
			log.Fatalf("Error reading pattern file: %s", err)
		}
	} else if !cfg.Empty {
		g.InitializeRandomCells()
	}

	var err error
	if g.birth, g.stayAlive, err = ParseRulestring(cfg.Rule); err != nil {
		log.Fatalf("Failed to parse the rule string (%s): %s\n", cfg.Rule, err)
	}

	// Draw initial world
	g.Draw("")
}

// TranslateXY move the x, y coordinates so that 0, 0 is the center of the world
// and handle wrapping at the edges
func (g *LifeGame) TranslateXY(x, y int) (int, int) {
	// Move x, y to center of field and wrap at the edges
	// NOTE: % in go preserves sign of a, unlike Python :)
	x = g.columns/2 + x
	x = (x%g.columns + g.columns) % g.columns
	y = g.rows/2 + y
	y = (y%g.rows + g.rows) % g.rows

	return x, y
}

// SetCellState sets the cell alive state
// it also wraps the x and y at the edges and returns the new value
func (g *LifeGame) SetCellState(x, y int, alive bool) (int, int) {
	x = x % g.columns
	y = y % g.rows
	g.cells[y][x].alive = alive
	g.cells[y][x].aliveNext = alive

	if !alive {
		g.cells[y][x].age = 0
	}

	return x, y
}

// PrintCellDetails prints the details for a cell, located by the window coordinates x, y
func (g *LifeGame) PrintCellDetails(x, y int32) {
	cellX := int(x) / cfg.CellSize
	cellY := int(y) / cfg.CellSize

	if cellX >= g.columns || cellY >= g.rows {
		log.Printf("ERROR: x=%d mapped to %d\n", x, cellX)
		log.Printf("ERROR: y=%d mapped to %d\n", y, cellY)
		return
	}

	log.Printf("%d, %d = %#v\n", cellX, cellY, g.cells[cellY][cellX])
}

// FillDead makes sure the rest of a line, width long, is filled with dead cells
// xEdge is the left side of the box of width length
// x is the starting point for the first line, any further lines start at xEdge
func (g *LifeGame) FillDead(xEdge, x, y, width, height int) {
	for i := 0; i < height; i++ {
		jlen := width - x
		for j := 0; j < jlen; j++ {
			x, y = g.SetCellState(x, y, false)
			x++
		}
		y++
		x = xEdge
	}
}

// InitializeRandomCells resets the world to a random state
func (g *LifeGame) InitializeRandomCells() {

	if cfg.Seed == 0 {
		seed := time.Now().UnixNano()
		log.Printf("seed = %d\n", seed)
		rand.Seed(seed)
	} else {
		log.Printf("seed = %d\n", cfg.Seed)
		rand.Seed(cfg.Seed)
	}

	for y := 0; y < g.rows; y++ {
		for x := 0; x < g.columns; x++ {
			g.SetCellState(x, y, rand.Float64() < threshold)
		}
	}
}

// ParseLife105 pattern file
// #D Descriptions lines (0+)
// #R Rule line (0/1)
// #P -1 4 (Upper left corner, required, center is 0,0)
// The pattern is . for dead and * for live
func (g *LifeGame) ParseLife105(lines []string) error {
	var x, y int
	var err error
	for _, line := range lines {
		if strings.HasPrefix(line, "#D") || strings.HasPrefix(line, "#Life") {
			continue
		} else if strings.HasPrefix(line, "#N") {
			// Use default rules (from the cmdline in this case)
			continue
		} else if strings.HasPrefix(line, "#R ") {
			// TODO Parse rule and return it or setup cfg.Rule
			// Format is: sss/bbb where s is stay alive and b are birth values
			// Need to flip it to Bbbb/Ssss format

			// Make sure the rule has a / in it
			if !strings.Contains(line, "/") {
				return fmt.Errorf("ERROR: Rule must contain /")
			}

			fields := strings.Split(line[3:], "/")
			if len(fields) != 2 {
				return fmt.Errorf("ERROR: Problem splitting rule on /")
			}

			var stay, birth int
			if stay, err = strconv.Atoi(fields[0]); err != nil {
				return fmt.Errorf("Error parsing alive value: %s", err)
			}

			if birth, err = strconv.Atoi(fields[1]); err != nil {
				return fmt.Errorf("Error parsing birth value: %s", err)
			}

			cfg.Rule = fmt.Sprintf("B%d/S%d", birth, stay)
		} else if strings.HasPrefix(line, "#P") {
			// Initial position
			fields := strings.Split(line, " ")
			if len(fields) != 3 {
				return fmt.Errorf("Cannot parse position line: %s", line)
			}
			if y, err = strconv.Atoi(fields[1]); err != nil {
				return fmt.Errorf("Error parsing position: %s", err)
			}
			if x, err = strconv.Atoi(fields[2]); err != nil {
				return fmt.Errorf("Error parsing position: %s", err)
			}

			// Move to 0, 0 at the center of the world
			x, y = g.TranslateXY(x, y)
		} else {
			// Parse the line, error if it isn't . or *
			xLine := x
			for _, c := range line {
				if c != '.' && c != '*' {
					return fmt.Errorf("Illegal characters in pattern: %s", line)
				}
				xLine, y = g.SetCellState(xLine, y, c == '*')
				xLine++
			}
			y++
		}
	}
	return nil
}

// ParsePlaintext pattern file
// The header has already been read from the buffer when this is called
// This is a bit more generic than the spec, skip lines starting with !
// and assume the pattern is . for dead cells any anything else for live.
func (g *LifeGame) ParsePlaintext(lines []string) error {
	var x, y int

	// Move x, y to center of field
	x = g.columns / 2
	y = g.rows / 2

	for _, line := range lines {
		if strings.HasPrefix(line, "!") {
			continue
		} else {
			// Parse the line, . is dead, anything else is alive.
			xLine := x
			for _, c := range line {
				g.SetCellState(xLine, y, c != '.')
				xLine++
			}
			y++
		}
	}

	return nil
}

// isRLEPattern checks the lines to determine if it is a RLE pattern
func isRLE(lines []string) bool {
	for _, line := range lines {
		if rleHeaderRegex.MatchString(line) {
			return true
		}
	}
	return false
}

// ParseRLE pattern file
// Parses files matching the RLE specification - https://conwaylife.com/wiki/Run_Length_Encoded
// Optional x, y starting position for later use
func (g *LifeGame) ParseRLE(lines []string, x, y int) error {
	// Move to 0, 0 at the center of the world
	x, y = g.TranslateXY(x, y)

	var header []string
	var first int
	for i, line := range lines {
		header = rleHeaderRegex.FindStringSubmatch(line)
		if len(header) > 0 {
			first = i + 1
			break
		}
		// All lines before the header must be a # line
		if line[0] != '#' {
			return fmt.Errorf("Incorrect or missing RLE header")
		}
	}
	if len(header) < 3 {
		return fmt.Errorf("Incorrect or missing RLE header")
	}
	if first > len(lines)-1 {
		return fmt.Errorf("Missing lines after RLE header")
	}
	width, err := strconv.Atoi(header[1])
	if err != nil {
		return fmt.Errorf("Error parsing width: %s", err)
	}
	height, err := strconv.Atoi(header[2])
	if err != nil {
		return fmt.Errorf("Error parsing height: %s", err)
	}

	// Were there rules? Use them. TODO override if cmdline rules passed in
	if len(header) == 4 {
		cfg.Rule = header[3]
	}

	count := 0
	xLine := x
	yStart := y
	for _, line := range lines[first:] {
		for _, c := range line {
			if c == '$' {
				// End of this line (which can have a count)
				if count == 0 {
					count = 1
				}
				// Blank cells to the edge of the pattern, and full empty lines
				g.FillDead(x, xLine, y, width, count)

				xLine = x
				y = y + count
				count = 0
				continue
			}
			if c == '!' {
				// Finished
				// Fill in any remaining space with dead cells
				g.FillDead(x, xLine, y, width, height-(y-yStart))
				return nil
			}

			// Is it a digit?
			digit, err := strconv.Atoi(string(c))
			if err == nil {
				count = (count * 10) + digit
				continue
			}

			if count == 0 {
				count = 1
			}

			for i := 0; i < count; i++ {
				xLine, y = g.SetCellState(xLine, y, c != 'b')
				xLine++
			}
			count = 0
		}
	}
	return nil
}

// checkState determines the state of the cell for the next tick of the game.
func (g *LifeGame) checkState(c *Cell) {
	liveCount, avgAge := g.liveNeighbors(c)
	if c.alive {
		// Stay alive if the number of neighbors is in stayAlive
		_, c.aliveNext = g.stayAlive[liveCount]
	} else {
		// Birth a new cell if number of neighbors is in birth
		_, c.aliveNext = g.birth[liveCount]

		// New cells inherit their age from parents
		// TODO make this optional
		if c.aliveNext {
			c.age = avgAge
		}
	}

	if c.aliveNext {
		c.age++
	} else {
		c.age = 0
	}
}

// liveNeighbors returns the number of live neighbors for a cell and their average age
func (g *LifeGame) liveNeighbors(c *Cell) (int, int) {
	var liveCount int
	var ageSum int
	add := func(x, y int) {
		// If we're at an edge, check the other side of the board.
		if y == len(g.cells) {
			y = 0
		} else if y == -1 {
			y = len(g.cells) - 1
		}
		if x == len(g.cells[y]) {
			x = 0
		} else if x == -1 {
			x = len(g.cells[y]) - 1
		}

		if g.cells[y][x].alive {
			liveCount++
			ageSum += g.cells[y][x].age
		}
	}

	add(c.x-1, c.y)   // To the left
	add(c.x+1, c.y)   // To the right
	add(c.x, c.y+1)   // up
	add(c.x, c.y-1)   // down
	add(c.x-1, c.y+1) // top-left
	add(c.x+1, c.y+1) // top-right
	add(c.x-1, c.y-1) // bottom-left
	add(c.x+1, c.y-1) // bottom-right

	if liveCount > 0 {
		return liveCount, int(ageSum / liveCount)
	}
	return liveCount, 0
}

// SetColorFromAge uses the cell's age to color it
func (g *LifeGame) SetColorFromAge(age int) {
	if age >= len(g.gradient.points) {
		age = len(g.gradient.points) - 1
	}
	color := g.gradient.points[age]
	g.renderer.SetDrawColor(color.r, color.g, color.b, color.a)
}

// Draw draws the current state of the world
func (g *LifeGame) Draw(status string) {
	// Clear the world to the background color
	g.renderer.SetDrawColor(g.bg.r, g.bg.g, g.bg.b, g.bg.a)
	g.renderer.Clear()
	g.renderer.SetDrawColor(g.fg.r, g.fg.g, g.fg.b, g.fg.a)
	for y := range g.cells {
		for _, c := range g.cells[y] {
			c.alive = c.aliveNext
			if !c.alive {
				continue
			}
			if cfg.Color {
				g.SetColorFromAge(c.age)
			}
			g.DrawCell(*c)
		}
	}
	// Default to background color
	g.renderer.SetDrawColor(g.bg.r, g.bg.g, g.bg.b, g.bg.a)

	g.UpdateStatus(status)

	g.renderer.Present()
}

// DrawCell draws a new cell on an empty background
func (g *LifeGame) DrawCell(c Cell) {
	var x, y int32
	if cfg.Rotate == 0 {
		if cfg.StatusTop {
			y = int32(c.y*cfg.CellSize + 4 + g.font.Height())
		} else {
			y = int32(c.y * cfg.CellSize)
		}
		x = int32(c.x * cfg.CellSize)
	} else if cfg.Rotate == 180 {
		// Invert top and bottom
		if cfg.StatusTop {
			y = int32(c.y * cfg.CellSize)
		} else {
			y = int32(c.y*cfg.CellSize + 4 + g.font.Height())
		}
		x = int32(c.x * cfg.CellSize)
	}

	if cfg.Border {
		g.renderer.FillRect(&sdl.Rect{x + 1, y + 1, int32(cfg.CellSize - 2), int32(cfg.CellSize - 2)})
	} else {
		g.renderer.FillRect(&sdl.Rect{x, y, int32(cfg.CellSize), int32(cfg.CellSize)})
	}
}

// UpdateCell redraws an existing cell, optionally erasing it
func (g *LifeGame) UpdateCell(x, y int, erase bool) {
	g.cells[y][x].alive = !erase

	// Update the image right now
	if erase {
		g.renderer.SetDrawColor(g.bg.r, g.bg.g, g.bg.b, g.bg.a)
	} else {
		g.renderer.SetDrawColor(g.fg.r, g.fg.g, g.fg.b, g.fg.a)
	}
	g.DrawCell(*g.cells[y][x])

	// Default to background color
	g.renderer.SetDrawColor(g.bg.r, g.bg.g, g.bg.b, g.bg.a)
	g.renderer.Present()
}

// UpdateStatus draws the status bar
func (g *LifeGame) UpdateStatus(status string) {
	if len(status) == 0 {
		return
	}
	text, err := g.font.RenderUTF8Solid(status, sdl.Color{255, 255, 255, 255})
	if err != nil {
		log.Printf("Failed to render text: %s\n", err)
		return
	}
	defer text.Free()

	texture, err := g.renderer.CreateTextureFromSurface(text)
	if err != nil {
		log.Printf("Failed to render text: %s\n", err)
		return
	}
	defer texture.Destroy()

	w, h, err := g.font.SizeUTF8(status)
	if err != nil {
		log.Printf("Failed to get size: %s\n", err)
		return
	}

	if cfg.Rotate == 0 {
		var y int32
		if cfg.StatusTop {
			y = 2
		} else {
			y = int32(cfg.Height - 2 - h)
		}

		x := int32((cfg.Width - w) / 2)
		rect := &sdl.Rect{x, y, int32(w), int32(h)}
		if err = g.renderer.Copy(texture, nil, rect); err != nil {
			log.Printf("Failed to copy texture: %s\n", err)
			return
		}
	} else if cfg.Rotate == 180 {
		var y int32
		if cfg.StatusTop {
			y = int32(cfg.Height - 2 - h)
		} else {
			y = 2
		}

		x := int32((cfg.Width - w) / 2)
		rect := &sdl.Rect{x, y, int32(w), int32(h)}
		if err = g.renderer.CopyEx(texture, nil, rect, 0.0, nil, sdl.FLIP_HORIZONTAL|sdl.FLIP_VERTICAL); err != nil {
			log.Printf("Failed to copy texture: %s\n", err)
			return
		}
	}
}

// NextFrame executes the next screen of the game
func (g *LifeGame) NextFrame() {
	last := g.liveCells
	g.liveCells = 0
	for y := range g.cells {
		for _, c := range g.cells[y] {
			g.checkState(c)
			if c.aliveNext {
				g.liveCells++
			}
		}
	}
	if g.liveCells-last != 0 {
		g.age++
	}

	// Draw a new screen
	status := fmt.Sprintf("age: %5d alive: %5d change: %5d", g.age, g.liveCells, g.liveCells-last)
	g.Draw(status)
}

// ShowKeysHelp prints the keys that are reconized to control behavior
func ShowKeysHelp() {
	fmt.Println("h           - Print help")
	fmt.Println("<space>     - Toggle pause/play")
	fmt.Println("c           - Toggle color")
	fmt.Println("q           - Quit")
	fmt.Println("s           - Single step")
	fmt.Println("r           - Reset the game")
}

// Run executes the main loop of the game
// it handles user input and updating the display at the selected update rate
func (g *LifeGame) Run() {
	fpsTime := sdl.GetTicks()

	running := true
	oneStep := false
	pause := cfg.Pause
	for running {
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch t := event.(type) {
			case *sdl.QuitEvent:
				running = false
				break
			case *sdl.KeyboardEvent:
				if t.GetType() == sdl.KEYDOWN {
					switch t.Keysym.Sym {
					case sdl.K_h:
						ShowKeysHelp()
					case sdl.K_q:
						running = false
						break
					case sdl.K_SPACE:
						pause = !pause
					case sdl.K_s:
						pause = true
						oneStep = true
					case sdl.K_r:
						g.InitializeCells()
					case sdl.K_c:
						cfg.Color = !cfg.Color
					}

				}
			case *sdl.MouseButtonEvent:
				if t.GetType() == sdl.MOUSEBUTTONDOWN {
					// log.Printf("x=%d y=%d\n", t.X, t.Y)
					g.PrintCellDetails(t.X, t.Y)
				}
			case *sdl.MouseMotionEvent:
				if t.GetType() == sdl.MOUSEMOTION {
					fmt.Printf("Motion Event (%d): ", t.Which)
					g.PrintCellDetails(t.X, t.Y)
				}

			case *sdl.MouseWheelEvent:
				if t.GetType() == sdl.MOUSEWHEEL {
					fmt.Printf("Wheel Event (%d): ", t.Which)
					g.PrintCellDetails(t.X, t.Y)
				}

			case *sdl.MultiGestureEvent:
				if t.GetType() == sdl.MULTIGESTURE {
					fmt.Printf("x=%0.2f y=%0.2f fingers=%d pinch=%0.2f rotate=%0.2f\n", t.X, t.Y, t.NumFingers, t.DDist, t.DTheta)
				}
			case *sdl.TouchFingerEvent:
				if t.GetType() == sdl.FINGERDOWN {
					fmt.Printf("x=%02.f y=%02.f dx=%02.f dy=%0.2f pressure=%0.2f\n", t.X, t.Y, t.DX, t.DY, t.Pressure)
				}
			}
		}
		// Delay a small amount
		time.Sleep(1 * time.Millisecond)
		if sdl.GetTicks() > fpsTime+(1000/uint32(cfg.Fps)) {
			if !pause || oneStep {
				g.NextFrame()
				fpsTime = sdl.GetTicks()
				oneStep = false
			}
		}

		if g.pChan != nil {
			select {
			case pattern := <-g.pChan:
				var err error
				if strings.HasPrefix(pattern[0], "#Life 1.05") {
					err = g.ParseLife105(pattern)
				} else if isRLE(pattern) {
					err = g.ParseRLE(pattern, 0, 0)
				} else {
					err = g.ParsePlaintext(pattern)
				}
				if err != nil {
					log.Printf("Pattern error: %s\n", err)
				}
			default:
			}
		}
	}
}

// CalculateWorldSize determines the most rows/columns to fit the world
func (g *LifeGame) CalculateWorldSize() {
	if cfg.Rotate == 0 || cfg.Rotate == 180 {
		// The status text is subtracted from the height
		g.columns = cfg.Width / cfg.CellSize
		g.rows = (cfg.Height - 4 - g.font.Height()) / cfg.CellSize
	} else if cfg.Rotate == 90 || cfg.Rotate == 270 {
		// The status text is subtracted from the width
		g.columns = (cfg.Width - 4 - g.font.Height()) / cfg.CellSize
		g.rows = cfg.Height / cfg.CellSize
	} else {
		log.Fatal("Unsupported rotate value")
	}
}

// InitializeGame sets up the game struct and the SDL library
// It also creates the main window
func InitializeGame() *LifeGame {
	game := &LifeGame{}

	var err error
	if err = sdl.Init(sdl.INIT_EVERYTHING); err != nil {
		log.Fatalf("Problem initializing SDL: %s", err)
	}

	if err = ttf.Init(); err != nil {
		log.Fatalf("Failed to initialize TTF: %s\n", err)
	}

	if game.font, err = ttf.OpenFont(cfg.Font, cfg.FontSize); err != nil {
		log.Fatalf("Failed to open font: %s\n", err)
	}
	log.Printf("Font height is %d", game.font.Height())

	game.font.SetHinting(ttf.HINTING_NORMAL)
	game.font.SetKerning(true)

	game.window, err = sdl.CreateWindow(
		"Conway's Game of Life",
		sdl.WINDOWPOS_UNDEFINED,
		sdl.WINDOWPOS_UNDEFINED,
		int32(cfg.Width),
		int32(cfg.Height),
		sdl.WINDOW_SHOWN)
	if err != nil {
		log.Fatalf("Problem initializing SDL window: %s", err)
	}

	game.renderer, err = sdl.CreateRenderer(game.window, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		log.Fatalf("Problem initializing SDL renderer: %s", err)
	}

	// White on Black background
	game.bg = RGBAColor{0, 0, 0, 255}
	game.fg = RGBAColor{255, 255, 255, 255}

	// Calculate the number of rows and columns that will fit
	game.CalculateWorldSize()

	// Parse the hex triplets
	colors, err := ParseColorTriplets(cfg.Colors)
	if err != nil {
		log.Fatalf("Problem parsing colors: %s", err)
	}

	// Build the color gradient
	switch cfg.Gradient {
	case LinearGradient:
		game.gradient, err = NewLinearGradient(colors, cfg.MaxAge)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}
	case PolylinearGradient:
		game.gradient, err = NewPolylinearGradient(colors, cfg.MaxAge)
		if err != nil {
			log.Fatalf("ERROR: %s", err)
		}
	case BezierGradient:
		game.gradient = NewBezierGradient(colors, cfg.MaxAge)
	}

	return game
}

// ParseColorTriplets parses color hex values into an array
// like ffffff,000000 or #ffffff,#000000 or #ffffff#000000
func ParseColorTriplets(s string) ([]RGBAColor, error) {

	// Remove leading # if present
	s = strings.TrimPrefix(s, "#")
	// Replace ,# combinations with just ,
	s = strings.ReplaceAll(s, ",#", ",")
	// Replace # alone with ,
	s = strings.ReplaceAll(s, "#", ",")

	var colors []RGBAColor
	// Convert the tuples into RGBAColor
	for _, c := range strings.Split(s, ",") {
		r, err := strconv.ParseUint(c[:2], 16, 8)
		if err != nil {
			return colors, err
		}
		g, err := strconv.ParseUint(c[2:4], 16, 8)
		if err != nil {
			return colors, err
		}
		b, err := strconv.ParseUint(c[4:6], 16, 8)
		if err != nil {
			return colors, err
		}

		colors = append(colors, RGBAColor{uint8(r), uint8(g), uint8(b), 255})
	}

	return colors, nil
}

// Parse digits into a map of ints from 0-9
//
// Returns an error if they aren't digits, or if there are more than 10 of them
func parseDigits(digits string) (map[int]bool, error) {
	ruleMap := make(map[int]bool, 10)

	var errors bool
	var err error
	var value int
	if len(digits) > 10 {
		log.Printf("%d has more than 10 digits", value)
		errors = true
	}
	if value, err = strconv.Atoi(digits); err != nil {
		log.Printf("%s must be digits from 0-9\n", digits)
		errors = true
	}
	if errors {
		return nil, fmt.Errorf("ERROR: Problem parsing digits")
	}

	// Add the digits to the map (order doesn't matter)
	for value > 0 {
		ruleMap[value%10] = true
		value = value / 10
	}

	return ruleMap, nil
}

// ParseRulestring parses the rules that control the game
//
// Rulestrings are of the form Bn.../Sn... which list the number of neighbors to birth a new one,
// and the number of neighbors to stay alive.
//
func ParseRulestring(rule string) (birth map[int]bool, stayAlive map[int]bool, e error) {
	var errors bool

	// Make sure the rule starts with a B
	if !strings.HasPrefix(rule, "B") {
		log.Println("ERROR: Rule must start with a 'B'")
		errors = true
	}

	// Make sure the rule has a /S in it
	if !strings.Contains(rule, "/S") {
		log.Println("ERROR: Rule must contain /S")
		errors = true
	}
	if errors {
		return nil, nil, fmt.Errorf("The Rule string should look similar to: B2/S23")
	}

	// Split on the / returning 2 results like Bnn and Snn
	fields := strings.Split(rule, "/")
	if len(fields) != 2 {
		return nil, nil, fmt.Errorf("ERROR: Problem splitting rule on /")
	}

	var err error
	// Convert the values to maps
	birth, err = parseDigits(strings.TrimPrefix(fields[0], "B"))
	if err != nil {
		errors = true
	}
	stayAlive, err = parseDigits(strings.TrimPrefix(fields[1], "S"))
	if err != nil {
		errors = true
	}
	if errors {
		return nil, nil, fmt.Errorf("ERROR: Problem with Birth or Stay alive values")
	}

	return birth, stayAlive, nil
}

// Server starts an API server to receive patterns
func Server(host string, port int, pChan chan<- Pattern) {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "", http.StatusMethodNotAllowed)
			return
		}

		scanner := bufio.NewScanner(r.Body)
		var pattern Pattern
		for scanner.Scan() {
			pattern = append(pattern, scanner.Text())
		}
		if len(pattern) == 0 {
			http.Error(w, "Empty pattern", http.StatusServiceUnavailable)
			return
		}

		// Splat this pattern onto the world
		pChan <- pattern
	})

	log.Printf("Starting server on %s:%d", host, port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", host, port), nil))
}

func main() {
	parseArgs()

	// If the user didn't specify a font, try to find a default one
	if len(cfg.Font) == 0 {
		for _, f := range defaultFonts {
			if _, err := os.Stat(f); !os.IsNotExist(err) {
				cfg.Font = f
				break
			}
		}
	}

	if len(cfg.Font) == 0 {
		log.Fatal("Failed to find a font for the statusbar. Use -font to Pass the path to a monospaced font")
	}

	// Initialize the main window
	game := InitializeGame()
	defer game.cleanup()

	// TODO
	// * resize events?
	// * add a status bar (either add to height, or subtract from it)

	// Setup the initial state of the world
	game.InitializeCells()

	ShowKeysHelp()

	if cfg.Server {
		ch := make(chan Pattern, 2)
		game.pChan = ch
		go Server(cfg.Host, cfg.Port, ch)
	}

	game.Run()
}
