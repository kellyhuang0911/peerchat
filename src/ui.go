package src

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type UI struct {
	*ChatRoom
	TerminalApp *tview.Application
	MsgInputs chan string
	CmdInputs chan uicommand

	peerBox *tview.TextView
	messageBox *tview.TextView
	inputBox *tview.InputField
}

type uicommand struct {
	cmdtype string
	cmdarg  string
}

func NewUI(cr *ChatRoom) *UI {
	app := tview.NewApplication()

	cmdchan := make(chan uicommand)
	msgchan := make(chan string)

	messagebox := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() {
			app.Draw()
		})

	messagebox.
		SetBorder(true).
		SetBorderColor(tcell.ColorBlue).
		SetTitle(cr.RoomName).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite)

	peerbox := tview.NewTextView()

	peerbox.
		SetBorder(true).
		SetBorderColor(tcell.ColorBlue).
		SetTitle("Peers").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite)

	input := tview.NewInputField().
		SetLabel(cr.Username + " > ").
		SetLabelColor(tcell.ColorBlue).
		SetFieldWidth(0).
		SetFieldBackgroundColor(tcell.ColorBlack)

	input.SetBorder(true).
		SetBorderColor(tcell.ColorBlue).
		SetTitle("Input").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite).
		SetBorderPadding(0, 0, 1, 0)

	input.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}

		line := input.GetText()

		if len(line) == 0 {
			return
		}

		if strings.HasPrefix(line, "/") {
			cmdparts := strings.Split(line, " ")

			if len(cmdparts) == 1 {
				cmdparts = append(cmdparts, "")
			}

			cmdchan <- uicommand{cmdtype: cmdparts[0], cmdarg: cmdparts[1]}

		} else {
			msgchan <- line
		}

		input.SetText("")
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(messagebox, 0, 1, false).
			AddItem(peerbox, 20, 1, false),
			0, 8, false).
		AddItem(input, 3, 1, true)

	app.SetRoot(flex, true)

	return &UI{
		ChatRoom:    cr,
		TerminalApp: app,
		peerBox:     peerbox,
		messageBox:  messagebox,
		inputBox:    input,
		MsgInputs:   msgchan,
		CmdInputs:   cmdchan,
	}
}

func (ui *UI) Run() error {
	go ui.starteventhandler()

	defer ui.cancelCtx()
	return ui.TerminalApp.Run()
}

func (ui *UI) starteventhandler() {
	refreshticker := time.NewTicker(time.Second)
	defer refreshticker.Stop()

	for {
		select {

		case msg := <-ui.MsgInputs:
			ui.OutgoingMessages <- msg
			ui.display_selfmessage(msg)

		case cmd := <-ui.CmdInputs:
			go ui.handlecommand(cmd)

		case msg := <-ui.IncomingMessages:
			ui.display_chatmessage(msg)

		case log := <-ui.LogChannel:
			ui.display_logmessage(log)

		case <-refreshticker.C:
			ui.syncpeerbox()

		case <-ui.roomCtx.Done():
			return
		}
	}
}

func (ui *UI) handlecommand(cmd uicommand) {

	switch cmd.cmdtype {

	case "/q":
		ui.TerminalApp.Stop()
		return

	case "/r":
		if cmd.cmdarg == "" {
			ui.LogChannel <- logEntry{Prefix: "badcmd", Msg: "missing room name for command"}
		} else {
			ui.LogChannel <- logEntry{Prefix: "roomchange", Msg: fmt.Sprintf("joining new room '%s'", cmd.cmdarg)}

			oldchatroom := ui.ChatRoom

			newchatroom, err := JoinRoom(ui.NodeHost, ui.Username, cmd.cmdarg)
			if err != nil {
				ui.LogChannel <- logEntry{Prefix: "jumperr", Msg: fmt.Sprintf("could not change chat room - %s", err)}
				return
			}

			ui.ChatRoom = newchatroom
			time.Sleep(time.Second * 1)

			oldchatroom.Leave()

			ui.messageBox.Clear()
			ui.messageBox.SetTitle(fmt.Sprintf("ChatRoom-%s", ui.ChatRoom.RoomName))
		}

	case "/u":
		if cmd.cmdarg == "" {
			ui.LogChannel <- logEntry{Prefix: "badcmd", Msg: "missing user name for command"}
		} else {
			ui.UpdateUsername(cmd.cmdarg)
			ui.inputBox.SetLabel(ui.Username + " > ")
		}

	case "/send":
		if cmd.cmdarg == "" {
			ui.LogChannel <- logEntry{Prefix: "badcmd", Msg: "missing file name for command"}
		} else {
			err := ui.ChatRoom.SendFile(cmd.cmdarg)
			if err != nil {
				ui.LogChannel <- logEntry{Prefix: "error", Msg: fmt.Sprintf("Failed to send file: %s", err)}
			} else {
				ui.LogChannel <- logEntry{Prefix: "info", Msg: "File sent successfully!"}
			}
		}
	default:
		ui.LogChannel <- logEntry{Prefix: "badcmd", Msg: fmt.Sprintf("unsupported command - %s", cmd.cmdtype)}
	}
}

func (ui *UI) display_chatmessage(msg chatMsg) {
	prompt := fmt.Sprintf("[green]<%s>:[-]", msg.SenderName)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, msg.Text)
}

func (ui *UI) display_selfmessage(msg string) {
	prompt := fmt.Sprintf("[blue]<%s>:[-]", ui.Username)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, msg)
}

func (ui *UI) display_logmessage(log logEntry) {
	prompt := fmt.Sprintf("[yellow]<%s>:[-]", log.Prefix)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, log.Msg)
}

func (ui *UI) syncpeerbox() {
	peers := ui.GetPeers()

	ui.peerBox.Lock()
	ui.peerBox.Clear()
	ui.peerBox.Unlock()

	for _, p := range peers {
		peerid := p.Pretty()
		peerid = peerid[len(peerid)-8:]
		fmt.Fprintln(ui.peerBox, peerid)
	}

	ui.TerminalApp.Draw()
}