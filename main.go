package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lucasb-eyer/go-colorful"
)

// ═══════════════════════════════════════════════════════════════
//  wmatrix - Matrix rain for Windows Terminal
//  by QuantumDev
// ═══════════════════════════════════════════════════════════════

// ─── Conjuntos de caracteres ───
var (
	asciiSet    = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+-=[]{}|;:,.<>?")
	katakanaSet = []rune("アイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワヲンｦｧｨｩｪｫｬｭｮｯｰｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉﾊﾋﾌﾍﾎﾏﾐﾑﾒﾓﾔﾕﾖﾗﾘﾙﾚﾛﾜﾝ")
	binarySet   = []rune("01")
	numbersSet  = []rune("0123456789")
)

// ─── Firma ───
const signature = "by QuantumDev"

// ─── Configuración ───
type Config struct {
	CharMode   string
	Hue        float64
	Saturation float64
	Lightness  float64
	Speed      int
	Bold       bool
	Rainbow    bool
	Async      bool
	FPS        int
}

func defaultConfig() Config {
	return Config{
		CharMode:   "ascii",
		Hue:        120,
		Saturation: 1.0,
		Lightness:  0.5,
		Speed:      10,
		Bold:       true,
		Rainbow:    false,
		Async:      true,
		FPS:        30,
	}
}

// ─── Celda del grid ───
type Cell struct {
	Char       rune
	Brightness float64
	Age        int
}

// ─── Columna ───
type Column struct {
	Head   int
	Speed  float64
	Active bool
}

// ─── Estado de la firma ───
type SignatureState int

const (
	SigHidden SignatureState = iota
	SigGlitching
	SigResolved
	SigFading
)

type Signature struct {
	State      SignatureState
	Chars      []rune          // caracteres actuales mostrados
	Target     []rune          // "by QuantumDev"
	Timers     []int           // frame en el que cada char se resuelve
	Brightness float64
	Frame      int
	NextAppear int             // frame en el que vuelve a aparecer
}

func newSignature() *Signature {
	target := []rune(signature)
	chars := make([]rune, len(target))
	timers := make([]int, len(target))
	for i := range chars {
		chars[i] = randomGlyph()
		timers[i] = rand.Intn(20) + 5 // resuelve entre frame 5 y 25
	}
	return &Signature{
		State:      SigHidden,
		Chars:      chars,
		Target:     target,
		Timers:     timers,
		Brightness: 0,
		Frame:      0,
		NextAppear: rand.Intn(300) + 450, // aparece entre 450-750 frames (~15-25 seg a 30fps)
	}
}

func randomGlyph() rune {
	all := append(asciiSet, katakanaSet...)
	return all[rand.Intn(len(all))]
}

// ─── Estado del juego ───
type Game struct {
	Screen    tcell.Screen
	Config    Config
	Cols      int
	Rows      int
	Grid      [][]Cell
	Columns   []Column
	Chars     []rune
	Frame     int
	Running   bool
	RainbowO  float64
	Signature *Signature
}

func NewGame(screen tcell.Screen, cfg Config) *Game {
	w, h := screen.Size()
	g := &Game{
		Screen:    screen,
		Config:    cfg,
		Cols:      w,
		Rows:      h,
		Grid:      make([][]Cell, h),
		Columns:   make([]Column, w),
		Running:   true,
		Signature: newSignature(),
	}

	for y := range g.Grid {
		g.Grid[y] = make([]Cell, w)
		for x := range g.Grid[y] {
			g.Grid[y][x] = Cell{Char: ' ', Brightness: 0, Age: 0}
		}
	}

	for x := range g.Columns {
		g.Columns[x] = Column{
			Head:   -rand.Intn(h/2) - 1,
			Speed:  rand.Float64()*0.6 + 0.4,
			Active: rand.Float64() > 0.3,
		}
	}

	g.setCharset()
	return g
}

func (g *Game) setCharset() {
	switch g.Config.CharMode {
	case "katakana":
		g.Chars = katakanaSet
	case "mixed":
		g.Chars = append(asciiSet, katakanaSet...)
	case "binary":
		g.Chars = binarySet
	case "numbers":
		g.Chars = numbersSet
	default:
		g.Chars = asciiSet
	}
}

func (g *Game) randomChar() rune {
	return g.Chars[rand.Intn(len(g.Chars))]
}

func (g *Game) resize() {
	w, h := g.Screen.Size()
	g.Cols = w
	g.Rows = h

	oldGrid := g.Grid
	g.Grid = make([][]Cell, h)
	for y := range g.Grid {
		g.Grid[y] = make([]Cell, w)
		for x := range g.Grid[y] {
			if y < len(oldGrid) && x < len(oldGrid[y]) {
				g.Grid[y][x] = oldGrid[y][x]
			} else {
				g.Grid[y][x] = Cell{Char: ' ', Brightness: 0, Age: 0}
			}
		}
	}

	if len(g.Columns) != w {
		oldCols := g.Columns
		g.Columns = make([]Column, w)
		for x := 0; x < w && x < len(oldCols); x++ {
			g.Columns[x] = oldCols[x]
		}
		for x := len(oldCols); x < w; x++ {
			g.Columns[x] = Column{
				Head:   -rand.Intn(h/2) - 1,
				Speed:  rand.Float64()*0.6 + 0.4,
				Active: rand.Float64() > 0.3,
			}
		}
	}
}

// ─── Update ───
func (g *Game) update() {
	steps := max(1, g.Config.Speed/3)

	for s := 0; s < steps; s++ {
		for x := 0; x < g.Cols; x++ {
			col := &g.Columns[x]

			if !col.Active {
				if rand.Float64() < 0.005*float64(g.Config.Speed)/5.0 {
					col.Active = true
					col.Head = -rand.Intn(10) - 2
				}
				continue
			}

			speed := 1.0
			if g.Config.Async {
				speed = col.Speed
			}
			if (g.Frame+x)%max(1, int(4/speed)) != 0 {
				continue
			}

			col.Head++

			if col.Head > g.Rows+5 {
				col.Head = -rand.Intn(15) - 2
				if g.Config.Async {
					col.Speed = rand.Float64()*0.7 + 0.3
				}
				if rand.Float64() < 0.3 {
					col.Active = false
				}
			}

			headY := col.Head

			for y := 0; y < g.Rows; y++ {
				cell := &g.Grid[y][x]

				if y == headY && y >= 0 && y < g.Rows {
					cell.Char = g.randomChar()
					cell.Brightness = 1.0
					cell.Age = 0
				} else if y < headY && y >= 0 && y < g.Rows {
					cell.Age++
					decay := 0.08
					if !g.Config.Bold {
						decay = 0.12
					}
					cell.Brightness = max(0, cell.Brightness-decay)
					if rand.Float64() < 0.03 {
						cell.Char = g.randomChar()
					}
				}
			}
		}
	}

	if g.Config.Rainbow {
		g.RainbowO = math.Mod(g.RainbowO+0.5, 360)
	}

	g.updateSignature()
	g.Frame++
}

// ─── Firma: update ───
func (g *Game) updateSignature() {
	sig := g.Signature

	switch sig.State {
	case SigHidden:
		if g.Frame >= sig.NextAppear {
			sig.State = SigGlitching
			sig.Frame = 0
			// Resetear timers
			for i := range sig.Timers {
				sig.Timers[i] = rand.Intn(20) + 5
				sig.Chars[i] = randomGlyph()
			}
		}

	case SigGlitching:
		sig.Frame++
		allResolved := true
		for i := range sig.Chars {
			if sig.Frame < sig.Timers[i] {
				// Sigue glitchando
				if rand.Float64() < 0.4 {
					sig.Chars[i] = randomGlyph()
				}
				allResolved = false
			} else {
				// Se resolvió
				sig.Chars[i] = sig.Target[i]
			}
		}
		// Brillo sube suavemente hasta 0.72
		sig.Brightness = min(0.72, float64(sig.Frame)*0.025)

		if allResolved && sig.Frame > 30 {
			sig.State = SigResolved
			sig.Frame = 0
		}

	case SigResolved:
		sig.Frame++
		// Mantener visible por un rato (60 frames ~ 2 seg a 30fps)
		if sig.Frame > 60 {
			sig.State = SigFading
			sig.Frame = 0
		}

	case SigFading:
		sig.Frame++
		sig.Brightness = max(0, 0.72-float64(sig.Frame)*0.018)
		// Glitchear de vuelta a aleatorio mientras se desvanece
		for i := range sig.Chars {
			if rand.Float64() < 0.1 {
				sig.Chars[i] = randomGlyph()
			}
		}
		if sig.Brightness <= 0 {
			sig.State = SigHidden
			sig.Brightness = 0
			sig.NextAppear = g.Frame + rand.Intn(300) + 450
		}
	}
}

// ─── Draw ───
func (g *Game) draw() {
	for y := 0; y < g.Rows; y++ {
		for x := 0; x < g.Cols; x++ {
			cell := g.Grid[y][x]
			if cell.Brightness <= 0.01 {
				g.Screen.SetContent(x, y, ' ', nil, tcell.StyleDefault)
				continue
			}

			style := g.cellStyle(cell, x)
			g.Screen.SetContent(x, y, cell.Char, nil, style)
		}
	}

	// Dibujar firma
	g.drawSignature()

	g.Screen.Show()
}

func (g *Game) cellStyle(cell Cell, x int) tcell.Style {
	hue := g.Config.Hue
	if g.Config.Rainbow {
		hue = math.Mod(hue+g.RainbowO+float64(x)*2, 360)
	}

	sat := g.Config.Saturation
	light := g.Config.Lightness
	alpha := cell.Brightness

	if g.Config.Bold && cell.Brightness > 0.85 {
		light = math.Min(0.95, light+0.3)
		sat = math.Max(0, sat-0.15)
	} else {
		light = math.Max(0.15, light-float64(cell.Age)*0.015)
	}

	c := colorful.Hsl(hue, sat, light)
	r, gb, b := c.RGB255()

	if alpha < 1.0 {
		ar := uint8(float64(r) * alpha)
		ag := uint8(float64(gb) * alpha)
		ab := uint8(float64(b) * alpha)
		r, gb, b = ar, ag, ab
	}

	fg := tcell.NewRGBColor(int32(r), int32(gb), int32(b))
	style := tcell.StyleDefault.Foreground(fg)
	if g.Config.Bold && cell.Brightness > 0.85 {
		style = style.Bold(true)
	}
	return style
}

// ─── Dibujar firma ───
func (g *Game) drawSignature() {
	sig := g.Signature
	if sig.State == SigHidden || sig.Brightness <= 0.01 {
		return
	}

	// Posición: esquina inferior derecha, padding de 3 celdas
	textLen := len(sig.Chars)
	startX := g.Cols - textLen - 3
	startY := g.Rows - 2

	if startX < 0 {
		return // pantalla muy pequeña
	}

	// Color: mismo estilo "cabeza brillante" que las columnas
	hue := g.Config.Hue
	if g.Config.Rainbow {
		hue = math.Mod(hue+g.RainbowO, 360)
	}

	// Mismo tratamiento que la cabeza de columna (cell.Brightness > 0.85)
	sat := math.Max(0, g.Config.Saturation-0.15)
	light := math.Min(0.95, g.Config.Lightness+0.3) * sig.Brightness

	c := colorful.Hsl(hue, sat, light)
	r, gb, b := c.RGB255()
	fg := tcell.NewRGBColor(int32(r), int32(gb), int32(b))
	style := tcell.StyleDefault.Foreground(fg).Bold(true)

	for i, ch := range sig.Chars {
		g.Screen.SetContent(startX+i, startY, ch, nil, style)
	}
}

// ─── Run ───
func (g *Game) run() {
	frameDuration := time.Second / time.Duration(g.Config.FPS)
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	eventCh := make(chan tcell.Event, 10)
	go func() {
		for g.Running {
			ev := g.Screen.PollEvent()
			if ev != nil {
				select {
				case eventCh <- ev:
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
	}()

	for g.Running {
		select {
		case <-ticker.C:
			g.update()
			g.draw()

		case ev := <-eventCh:
			switch ev := ev.(type) {
			case *tcell.EventResize:
				g.Screen.Sync()
				g.resize()
			case *tcell.EventKey:
				if ev.Key() == tcell.KeyEscape || ev.Key() == tcell.KeyCtrlC {
					g.Running = false
					return
				}
				switch ev.Rune() {
				case 'q', 'Q':
					g.Running = false
					return
				case 'p', 'P':
					g.pause()
				case 'b', 'B':
					g.Config.Bold = !g.Config.Bold
				case 'r', 'R':
					g.Config.Rainbow = !g.Config.Rainbow
				case '1':
					g.Config.CharMode = "ascii"
					g.setCharset()
				case '2':
					g.Config.CharMode = "katakana"
					g.setCharset()
				case '3':
					g.Config.CharMode = "binary"
					g.setCharset()
				case '4':
					g.Config.CharMode = "numbers"
					g.setCharset()
				case '5':
					g.Config.CharMode = "mixed"
					g.setCharset()
				case '+', '=':
					g.Config.Speed = min(20, g.Config.Speed+1)
				case '-', '_':
					g.Config.Speed = max(1, g.Config.Speed-1)
				}
			}
		}
	}
}

func (g *Game) pause() {
	w, h := g.Screen.Size()
	msg := " PAUSADO - Pulsa cualquier tecla "
	x := (w - len(msg)) / 2
	y := h / 2
	for i, r := range msg {
		g.Screen.SetContent(x+i, y, r, nil, tcell.StyleDefault.Bold(true).Foreground(tcell.ColorWhite))
	}
	g.Screen.Show()

	for {
		ev := g.Screen.PollEvent()
		if ev != nil {
			if _, ok := ev.(*tcell.EventKey); ok {
				return
			}
		}
	}
}

// ─── Usage ───
func printUsage() {
	fmt.Println("wmatrix - Matrix rain for Windows Terminal")
	fmt.Println("by QuantumDev")
	fmt.Println()
	fmt.Println("Uso: wmatrix [opciones]")
	fmt.Println()
	fmt.Println("Opciones:")
	fmt.Println("  -mode string    Modo: ascii, katakana, mixed, binary, numbers (default: ascii)")
	fmt.Println("  -color float    Tono HSL 0-360 (default: 120 = verde)")
	fmt.Println("  -sat float      Saturación 0-1 (default: 1.0)")
	fmt.Println("  -light float    Brillo 0-1 (default: 0.5)")
	fmt.Println("  -speed int      Velocidad 1-20 (default: 10)")
	fmt.Println("  -bold           Cabeza brillante (default: true)")
	fmt.Println("  -rainbow        Cambio de color automático")
	fmt.Println("  -async          Columnas asíncronas (default: true)")
	fmt.Println("  -fps int        FPS máximo 15-60 (default: 30)")
	fmt.Println()
	fmt.Println("Controles:")
	fmt.Println("  Q / ESC / Ctrl+C   Salir")
	fmt.Println("  P                  Pausa")
	fmt.Println("  B                  Toggle bold")
	fmt.Println("  R                  Toggle rainbow")
	fmt.Println("  1-5                Cambiar modo de caracteres")
	fmt.Println("  + / -              Ajustar velocidad")
}

// ─── Main ───
func main() {
	rand.Seed(time.Now().UnixNano())

	cfg := defaultConfig()

	flag.StringVar(&cfg.CharMode, "mode", cfg.CharMode, "Modo: ascii, katakana, mixed, binary, numbers")
	flag.Float64Var(&cfg.Hue, "color", cfg.Hue, "Tono HSL 0-360")
	flag.Float64Var(&cfg.Saturation, "sat", cfg.Saturation, "Saturación 0-1")
	flag.Float64Var(&cfg.Lightness, "light", cfg.Lightness, "Brillo 0-1")
	flag.IntVar(&cfg.Speed, "speed", cfg.Speed, "Velocidad 1-20")
	flag.BoolVar(&cfg.Bold, "bold", cfg.Bold, "Cabeza brillante")
	flag.BoolVar(&cfg.Rainbow, "rainbow", cfg.Rainbow, "Cambio de color automático")
	flag.BoolVar(&cfg.Async, "async", cfg.Async, "Columnas asíncronas")
	flag.IntVar(&cfg.FPS, "fps", cfg.FPS, "FPS máximo 15-60")
	help := flag.Bool("h", false, "Mostrar ayuda")
	flag.Parse()

	if *help {
		printUsage()
		os.Exit(0)
	}

	switch cfg.CharMode {
	case "ascii", "katakana", "mixed", "binary", "numbers":
		// ok
	default:
		fmt.Fprintf(os.Stderr, "Modo desconocido: %s\n", cfg.CharMode)
		os.Exit(1)
	}

	s, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := s.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer s.Fini()

	s.SetStyle(tcell.StyleDefault)
	s.Clear()

	game := NewGame(s, cfg)
	game.run()
}
