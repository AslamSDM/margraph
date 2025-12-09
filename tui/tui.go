package tui

import (
	"fmt"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TUI represents the terminal user interface
type TUI struct {
	app          *tview.Application
	logsView     *tview.TextView
	inputField   *tview.InputField
	statsView    *tview.TextView
	headerView   *tview.TextView
	commandChan  chan string
	mu           sync.Mutex
	logBuffer    []string
	maxLogLines  int
}

// New creates a new TUI instance
func New() *TUI {
	t := &TUI{
		app:         tview.NewApplication(),
		commandChan: make(chan string, 10),
		logBuffer:   make([]string, 0),
		maxLogLines: 1000,
	}

	// Create header
	t.headerView = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("[::b]MARGRAF v1.0[::-] - Financial Dynamic Knowledge Graph").
		SetDynamicColors(true)
	t.headerView.SetBorder(true).SetBorderColor(tcell.ColorNames["blue"])

	// Create stats view
	t.statsView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	t.statsView.SetBorder(true).
		SetTitle(" Graph Statistics ").
		SetBorderColor(tcell.ColorNames["green"])
	t.UpdateStats(0, 0)

	// Create logs view
	t.logsView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			t.app.Draw()
		})
	t.logsView.SetBorder(true).
		SetTitle(" Logs ").
		SetBorderColor(tcell.ColorNames["yellow"])

	// Create input field
	t.inputField = tview.NewInputField().
		SetLabel("> ").
		SetFieldWidth(0).
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				command := t.inputField.GetText()
				if command != "" {
					t.commandChan <- command
					t.inputField.SetText("")
				}
			}
		})
	t.inputField.SetBorder(true).
		SetTitle(" Command Input (Press Enter to submit, Ctrl+C to quit) ").
		SetBorderColor(tcell.ColorNames["cyan"])

	// Create layout
	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.headerView, 3, 0, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(t.logsView, 0, 3, false).
			AddItem(t.statsView, 40, 0, false),
			0, 1, false).
		AddItem(t.inputField, 3, 0, true)

	t.app.SetRoot(mainFlex, true).SetFocus(t.inputField)

	return t
}

// Start starts the TUI application
func (t *TUI) Start() error {
	return t.app.Run()
}

// Stop stops the TUI application
func (t *TUI) Stop() {
	t.app.Stop()
}

// GetCommandChannel returns the channel for receiving commands
func (t *TUI) GetCommandChannel() <-chan string {
	return t.commandChan
}

// Log adds a log message to the logs view
func (t *TUI) Log(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Add to buffer
	t.logBuffer = append(t.logBuffer, message)

	// Keep only last N lines
	if len(t.logBuffer) > t.maxLogLines {
		t.logBuffer = t.logBuffer[len(t.logBuffer)-t.maxLogLines:]
	}

	// Update view
	t.app.QueueUpdateDraw(func() {
		t.logsView.Clear()
		for _, line := range t.logBuffer {
			fmt.Fprintln(t.logsView, line)
		}
		t.logsView.ScrollToEnd()
	})
}

// UpdateStats updates the statistics display
func (t *TUI) UpdateStats(nodeCount, edgeCount int) {
	t.app.QueueUpdateDraw(func() {
		t.statsView.Clear()
		fmt.Fprintf(t.statsView, "[green::b]Nodes:[-:-:-] %d\n", nodeCount)
		fmt.Fprintf(t.statsView, "[yellow::b]Edges:[-:-:-] %d\n", edgeCount)
		fmt.Fprintf(t.statsView, "\n[cyan]Status:[-] Running\n")
		fmt.Fprintf(t.statsView, "\n[white::b]Available Commands:[-:-:-]\n")
		fmt.Fprintln(t.statsView, "[gray]show, edges, discover[-]")
		fmt.Fprintln(t.statsView, "[gray]companies, relations[-]")
		fmt.Fprintln(t.statsView, "[gray]shock, boost, news[-]")
		fmt.Fprintln(t.statsView, "[gray]save, load, export[-]")
		fmt.Fprintln(t.statsView, "[gray]exit[-]")
	})
}

// SetHeader updates the header text
func (t *TUI) SetHeader(text string) {
	t.app.QueueUpdateDraw(func() {
		t.headerView.SetText(text)
	})
}

// GetApp returns the underlying tview application
func (t *TUI) GetApp() *tview.Application {
	return t.app
}

// Writer implements io.Writer for the TUI
type Writer struct {
	tui *TUI
}

// NewWriter creates a new TUI writer
func (t *TUI) NewWriter() *Writer {
	return &Writer{tui: t}
}

// Write implements io.Writer
func (w *Writer) Write(p []byte) (n int, err error) {
	message := string(p)
	// Remove trailing newline if present
	if len(message) > 0 && message[len(message)-1] == '\n' {
		message = message[:len(message)-1]
	}
	w.tui.Log(message)
	return len(p), nil
}
