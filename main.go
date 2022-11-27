// sdl2-life
// by Brian C. Lane <bcl@brianlane.com>
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
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
)

// RLE header with variable spacing and optional rules
// Matches it with or without rule at the end, and with 0 or more spaces between elements.
var rleHeaderRegex = regexp.MustCompile(`x\s*=\s*(\d+)\s*,\s*y\s*=\s*(\d+)(?:\s*,\s*rule\s*=\s*(.*))*`)

/* commandline flags */
type cmdlineArgs struct {
	Width       int    // Width of window in pixels
	Height      int    // Height of window in pixels
	Rows        int    // Number of cell rows
	Columns     int    // Number of cell columns
	Seed        int64  // Seed for PRNG
	Border      bool   // Border around cells
	Font        string // Path to TTF to use for status bar
	FontSize    int    // Size of font in points
	Rule        string // Rulestring to use
	Fps         int    // Frames per Second
	PatternFile string // File with initial pattern
	Pause       bool   // Start the game paused
	Empty       bool   // Start with empty world
	Port        int    // Port to listen to
	Host        string // Host IP to bind to
	Server      bool   // Launch an API server when true
}

/* commandline defaults */
var cfg = cmdlineArgs{
	Width:       500,
	Height:      500,
	Rows:        100,
	Columns:     100,
	Seed:        0,
	Border:      false,
	Font:        "",
	FontSize:    14,
	Rule:        "B3/S23",
	Fps:         10,
	PatternFile: "",
	Pause:       false,
	Empty:       false,
	Port:        3051,
	Host:        "127.0.0.1",
	Server:      false,
}

/* parseArgs handles parsing the cmdline args and setting values in the global cfg struct */
func parseArgs() {
	flag.IntVar(&cfg.Width, "width", cfg.Width, "Width of window in pixels")
	flag.IntVar(&cfg.Height, "height", cfg.Height, "Height of window in pixels")
	flag.IntVar(&cfg.Rows, "rows", cfg.Rows, "Number of cell rows")
	flag.IntVar(&cfg.Columns, "columns", cfg.Columns, "Number of cell columns")
	flag.Int64Var(&cfg.Seed, "seed", cfg.Seed, "PRNG seed")
	flag.BoolVar(&cfg.Border, "border", cfg.Border, "Border around cells")
	flag.StringVar(&cfg.Font, "font", cfg.Font, "Path to TTF to use for status bar")
	flag.IntVar(&cfg.FontSize, "font-size", cfg.FontSize, "Size of font in points")
	flag.StringVar(&cfg.Rule, "rule", cfg.Rule, "Rulestring Bn.../Sn... (B3/S23)")
	flag.IntVar(&cfg.Fps, "fps", cfg.Fps, "Frames per Second update rate (10fps)")
	flag.StringVar(&cfg.PatternFile, "pattern", cfg.PatternFile, "File with initial pattern to load")
	flag.BoolVar(&cfg.Pause, "pause", cfg.Pause, "Start the game paused")
	flag.BoolVar(&cfg.Empty, "empty", cfg.Empty, "Start with empty world")
	flag.IntVar(&cfg.Port, "port", cfg.Port, "Port to listen to")
	flag.StringVar(&cfg.Host, "host", cfg.Host, "Host IP to bind to")
	flag.BoolVar(&cfg.Server, "server", cfg.Server, "Launch an API server")

	flag.Parse()
}

// Possible default fonts to search for
var defaultFonts = []string{"/usr/share/fonts/liberation/LiberationMono-Regular.ttf",
	"/usr/local/share/fonts/TerminusTTF/TerminusTTF-4.49.2.ttf",
	"/usr/X11/share/fonts/TTF/LiberationMono-Regular.ttf"}

// RGBAColor holds a color
type RGBAColor struct {
	r, g, b, a uint8
}

// Cell describes the location and state of a cell
type Cell struct {
	alive     bool
	aliveNext bool

	x int
	y int
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
	window     *sdl.Window
	renderer   *sdl.Renderer
	font       *ttf.Font
	bg         RGBAColor
	fg         RGBAColor
	cellWidth  int32
	cellHeight int32
	pChan      <-chan Pattern
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
	g.cells = make([][]*Cell, cfg.Rows, cfg.Columns)
	for y := 0; y < cfg.Rows; y++ {
		for x := 0; x < cfg.Columns; x++ {
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

	for y := 0; y < cfg.Rows; y++ {
		for x := 0; x < cfg.Columns; x++ {
			g.cells[y][x].alive = rand.Float64() < threshold
			g.cells[y][x].aliveNext = g.cells[y][x].alive
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

			// Move x, y to center of field and wrap at the edges
			// NOTE: % in go preserves sign of a, unlike Python :)
			x = cfg.Columns/2 + x
			x = (x%cfg.Columns + cfg.Columns) % cfg.Columns
			y = cfg.Rows/2 + y
			y = (y%cfg.Rows + cfg.Rows) % cfg.Rows
		} else {
			// Parse the line, error if it isn't . or *
			xLine := x
			for _, c := range line {
				if c != '.' && c != '*' {
					return fmt.Errorf("Illegal characters in pattern: %s", line)
				}
				g.cells[y][xLine].alive = bool(c == '*')
				g.cells[y][xLine].aliveNext = bool(c == '*')
				xLine = (xLine + 1) % cfg.Columns
			}
			y = (y + 1) % cfg.Rows
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
	x = cfg.Columns / 2
	y = cfg.Rows / 2

	for _, line := range lines {
		if strings.HasPrefix(line, "!") {
			continue
		} else {
			// Parse the line, . is dead, anything else is alive.
			xLine := x
			for _, c := range line {
				g.cells[y][xLine].alive = bool(c != '.')
				g.cells[y][xLine].aliveNext = bool(c != '.')
				xLine = (xLine + 1) % cfg.Columns
			}
			y = (y + 1) % cfg.Rows
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
	// Move x, y to center of field and wrap at the edges
	// NOTE: % in go preserves sign of a, unlike Python :)
	x = cfg.Columns/2 + x
	x = (x%cfg.Columns + cfg.Columns) % cfg.Columns
	y = cfg.Rows/2 + y
	y = (y%cfg.Rows + cfg.Rows) % cfg.Rows

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

	// TODO Parse rules from header[3] and alter the game

	count := 0
	xLine := x
	yStart := y
	for _, line := range lines[first:] {
		for _, c := range line {
			if c == '$' {
				// End of this line (which can have a count)
				eols := 0
				if count == 0 {
					eols = 1
				} else {
					eols = count
				}

				// Blank cells to the edge of the pattern, and full empty lines
				for i := 0; i < eols; i++ {
					for j := xLine; j < x+width; j++ {
						g.cells[y][j].alive = false
						g.cells[y][j].aliveNext = false
					}
					// 2nd line and more fill the full line
					xLine = x
					y = y + 1
				}

				count = 0
				xLine = x
				continue
			}
			if c == '!' {
				// Finished

				// Fill in any remaining space with dead cells
				eols := height - (y - yStart)
				// Blank cells to the edge of the pattern, and full empty lines
				for i := 0; i < eols; i++ {
					for j := xLine; j < x+width; j++ {
						g.cells[y][j].alive = false
						g.cells[y][j].aliveNext = false
					}
					// 2nd line and more fill the full line
					xLine = x
					y = y + 1
				}

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

			// TODO wrap at edges
			for i := 0; i < count; i++ {
				g.cells[y][xLine].alive = bool(c != 'b')
				g.cells[y][xLine].aliveNext = bool(c != 'b')
				xLine++
			}
			count = 0
		}
	}
	return nil
}

// checkState determines the state of the cell for the next tick of the game.
func (g *LifeGame) checkState(c *Cell) {
	liveCount := g.liveNeighbors(c)
	if c.alive {
		// Stay alive if the number of neighbors is in stayAlive
		_, c.aliveNext = g.stayAlive[liveCount]
	} else {
		// Birth a new cell if number of neighbors is in birth
		_, c.aliveNext = g.birth[liveCount]
	}
}

// liveNeighbors returns the number of live neighbors for a cell.
func (g *LifeGame) liveNeighbors(c *Cell) int {
	var liveCount int
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

	return liveCount
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
	x := int32(c.x) * g.cellWidth
	y := int32(c.y) * g.cellHeight
	if cfg.Border {
		g.renderer.FillRect(&sdl.Rect{x + 1, y + 1, g.cellWidth - 2, g.cellHeight - 2})
	} else {
		g.renderer.FillRect(&sdl.Rect{x, y, g.cellWidth, g.cellHeight})
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

	x := int32((cfg.Width - w) / 2)
	rect := &sdl.Rect{x, int32(cfg.Height + 2), int32(w), int32(h)}
	if err = g.renderer.Copy(texture, nil, rect); err != nil {
		log.Printf("Failed to copy texture: %s\n", err)
		return
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
					}

				}
			case *sdl.MouseButtonEvent:
				if t.GetType() == sdl.MOUSEBUTTONDOWN {
					log.Printf("x=%d y=%d\n", t.X, t.Y)
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
		int32(cfg.Height+4+game.font.Height()),
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

	// Calculate square cell size, take into account --border selection
	w := cfg.Width / cfg.Columns
	h := cfg.Height / cfg.Rows
	if w < h {
		h = w
	} else {
		w = h
	}
	game.cellWidth = int32(w)
	game.cellHeight = int32(h)

	return game
}

// Parse digits into a map of ints from 0-9
//
// Returns an error if they aren't digits, or if there are more than 10 of them
func parseDigits(digits string) (map[int]bool, error) {
	ruleMap := make(map[int]bool, 10)

	var errors bool
	var err error
	var value int
	if value, err = strconv.Atoi(digits); err != nil {
		log.Printf("%s must be digits from 0-9\n", digits)
		errors = true
	}
	if value > 9999999999 {
		log.Printf("%d has more than 10 digits", value)
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
