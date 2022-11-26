# sdl2-life

[Conway's Game of Life](https://www.conwaylife.com/wiki/Conway's_Game_of_Life)

This implementation uses a [SDL2 Go library](https://github.com/veandco/go-sdl2/) to draw the world.

* It currently supports loading [Life 1.05 pattern files](https://www.conwaylife.com/wiki/Life_1.05)
* Supports plaintext pattern files like those from the [Life Lexicon](https://www.conwaylife.com/ref/lexicon/lex_1.htm)
* Hit 'h' to display they key help on the console while it is running.
* Pass '-help' on the cmdline to see the available options.
* Pass '-empty' to start with an empty world, this is useful when combined with '-server' which normally starts
  with a random seed.

The window size (width, height) should be evenly divisible by columns and rows
respectively. Otherwise the cell size is rounded down, so that they are square,
and the world may not use all of the available space.

Resizing the window with a window manager is likely to also cause problems --
this code was written suing dwm and a floating window with no decorations so I
haven't added any resize support.

## Server

Passing '-server' will listen to port 3051 for pattern files to be POSTed to it. This supports the same formats
as the cmdline -pattern argument (Life 1.05 and plain text). You can easily send it a pattern file using curl like
this:

    curl --data-binary @./examples/glider-gun-1.05.life http://127.0.0.1:3051/

## Building

Run `go build`

The only dependency is on the [SDL2 Go library](https://github.com/veandco/go-sdl2/)
