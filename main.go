package main

import (
	"flag"
	"log"
	"math/rand"
	"time"

	"github.com/veandco/go-sdl2/sdl"
)

const (
	threshold    = 0.15
	fps          = 10
	statusHeight = 0
)

/* commandline flags */
type cmdlineArgs struct {
	Width   int   // Width of window in pixels
	Height  int   // Height of window in pixels
	Rows    int   // Number of cell rows
	Columns int   // Number of cell columns
	Seed    int64 // Seed for PRNG
	Border  bool  // Border around cells
}

/* commandline defaults */
var cfg = cmdlineArgs{
	Width:   500,
	Height:  500,
	Rows:    100,
	Columns: 100,
	Seed:    0,
	Border:  false,
}

/* parseArgs handles parsing the cmdline args and setting values in the global cfg struct */
func parseArgs() {
	flag.IntVar(&cfg.Width, "width", cfg.Width, "Width of window in pixels")
	flag.IntVar(&cfg.Height, "height", cfg.Height, "Height of window in pixels")
	flag.IntVar(&cfg.Rows, "rows", cfg.Rows, "Number of cell rows")
	flag.IntVar(&cfg.Columns, "columns", cfg.Columns, "Number of cell columns")
	flag.Int64Var(&cfg.Seed, "seed", cfg.Seed, "PRNG seed")
	flag.BoolVar(&cfg.Border, "border", cfg.Border, "Border around cells")

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
	pause     bool

	// Graphics
	window     *sdl.Window
	renderer   *sdl.Renderer
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
	sdl.Quit()
}

// InitializeCells resets the world to a random state
func (g *LifeGame) InitializeCells() {
	if cfg.Seed == 0 {
		seed := time.Now().UnixNano()
		log.Printf("seed = %d\n", cfg.Seed)
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
		// 1. Any live cell with fewer than two live neighbours dies, as if caused by underpopulation.
		if liveCount < 2 {
			c.aliveNext = false
		}

		// 2. Any live cell with two or three live neighbours lives on to the next generation.
		if liveCount == 2 || liveCount == 3 {
			c.aliveNext = true
		}

		// 3. Any live cell with more than three live neighbours dies, as if by overpopulation.
		if liveCount > 3 {
			c.aliveNext = false
		}
	} else {
		// 4. Any dead cell with exactly three live neighbours becomes a live cell, as if by reproduction.
		if liveCount == 3 {
			c.aliveNext = true
		}
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
func (g *LifeGame) Draw() {
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

// NextFrame executes the next screen of the game
func (g *LifeGame) NextFrame() {
	if g.pause {
		return
	}

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

	// TODO -- Implement new status bar
	//	statusbar.ShowMessage(fmt.Sprintf("age: %5d alive: %4d change: %d", age, liveCells, liveCells-last), 0)
	log.Printf("age: %5d alive: %4d change: %d", g.age, g.liveCells, g.liveCells-last)

	// Draw a new screen
	g.Draw()
}

// Run executes the main loop of the game
// it handles user input and updating the display at the selected update rate
func (g *LifeGame) Run() {
	fpsTime := sdl.GetTicks()
	g.Draw()

	running := true
	for running {
		// TODO This loop has very high CPU usage. How to fix that?
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
						g.pause = !g.pause
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
		if sdl.GetTicks() > fpsTime+(1000/fps) {
			g.NextFrame()
			fpsTime = sdl.GetTicks()
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

	// Calculate square cell size, take into account --border selection
	w := cfg.Width / cfg.Columns
	h := (cfg.Height - statusHeight) / cfg.Rows
	if w < h {
		h = w
	} else {
		w = h
	}
	game.cellWidth = int32(w)
	game.cellHeight = int32(h)

	return game
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
