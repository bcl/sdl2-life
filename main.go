package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

const (
	threshold = 0.15
)

/* commandline flags */
type cmdlineArgs struct {
	Width    int    // Width of window in pixels
	Height   int    // Height of window in pixels
	Rows     int    // Number of cell rows
	Columns  int    // Number of cell columns
	Seed     int64  // Seed for PRNG
	Border   bool   // Border around cells
	Font     string // Path to TTF to use for status bar
	FontSize int    // Size of font in points
	Rule     string // Rulestring to use
	Fps      int    // Frames per Second
}

/* commandline defaults */
var cfg = cmdlineArgs{
	Width:    500,
	Height:   500,
	Rows:     100,
	Columns:  100,
	Seed:     0,
	Border:   false,
	Font:     "/usr/share/fonts/liberation/LiberationMono-Regular.ttf",
	FontSize: 14,
	Rule:     "B3/S23",
	Fps:      10,
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

	flag.Parse()
}

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

// LifeGame holds all the global state of the game and the methods to operate on it
type LifeGame struct {
	mp        bool
	erase     bool
	cells     [][]*Cell
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

// InitializeCells resets the world to a random state
func (g *LifeGame) InitializeCells() {
	if cfg.Seed == 0 {
		seed := time.Now().UnixNano()
		log.Printf("seed = %d\n", seed)
		rand.Seed(seed)
	} else {
		log.Printf("seed = %d\n", cfg.Seed)
		rand.Seed(cfg.Seed)
	}

	g.age = 0
	g.cells = make([][]*Cell, cfg.Rows, cfg.Rows)
	for x := 0; x < cfg.Rows; x++ {
		for y := 0; y < cfg.Columns; y++ {
			c := &Cell{x: x, y: y}
			c.alive = rand.Float64() < threshold
			c.aliveNext = c.alive

			g.cells[x] = append(g.cells[x], c)
		}
	}
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
		if x == len(g.cells) {
			x = 0
		} else if x == -1 {
			x = len(g.cells) - 1
		}
		if y == len(g.cells[x]) {
			y = 0
		} else if y == -1 {
			y = len(g.cells[x]) - 1
		}

		if g.cells[x][y].alive {
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
	for x := range g.cells {
		for _, c := range g.cells[x] {
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
	g.cells[x][y].alive = !erase

	// Update the image right now
	if erase {
		g.renderer.SetDrawColor(g.bg.r, g.bg.g, g.bg.b, g.bg.a)
	} else {
		g.renderer.SetDrawColor(g.fg.r, g.fg.g, g.fg.b, g.fg.a)
	}
	g.DrawCell(*g.cells[x][y])

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
	for x := range g.cells {
		for _, c := range g.cells[x] {
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

// Run executes the main loop of the game
// it handles user input and updating the display at the selected update rate
func (g *LifeGame) Run() {
	fpsTime := sdl.GetTicks()

	running := true
	oneStep := false
	pause := false
	for running {
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch t := event.(type) {
			case *sdl.QuitEvent:
				running = false
				break
			case *sdl.KeyboardEvent:
				if t.GetType() == sdl.KEYDOWN {
					switch t.Keysym.Sym {
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
	}
}

// InitializeGame sets up the game struct and the SDL library
// It also creates the main window
func InitializeGame() *LifeGame {
	game := &LifeGame{}

	var err error
	if game.birth, game.stayAlive, err = ParseRulestring(cfg.Rule); err != nil {
		log.Fatalf("Failed to parse the rule string (%s): %s\n", cfg.Rule, err)
	}

	if err = sdl.Init(sdl.INIT_EVERYTHING); err != nil {
		log.Fatalf("Problem initializing SDL: %s", err)
	}

	if err = ttf.Init(); err != nil {
		log.Fatalf("Failed to initialize TTF: %s\n", err)
	}

	// TODO Better Font Selection
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

func main() {
	parseArgs()

	// Initialize the main window
	game := InitializeGame()
	defer game.cleanup()

	// TODO
	// * resize events?
	// * add a status bar (either add to height, or subtract from it)

	// Setup the initial state of the world
	game.InitializeCells()

	game.Run()
}
