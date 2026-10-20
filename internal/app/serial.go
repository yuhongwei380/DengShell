package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.bug.st/serial"
)

const serialRelayFrames = 16
const serialMaxSessions = 16

// serialState owns every open serial port. Like RDP it is independent from the
// SSH session map: a serial tab has no SFTP, monitoring or window handoff.
type serialState struct {
	mu       sync.Mutex
	sessions map[string]*serialSession
	closed   bool
}

type SerialInfo struct {
	ID          string `json:"id"`
	ProfileID   string `json:"profileId"`
	Name        string `json:"name"`
	SerialPort  string `json:"serialPort"`
	BaudRate    int    `json:"baudRate"`
	DataBits    int    `json:"dataBits"`
	Parity      string `json:"parity"`
	StopBits    int    `json:"stopBits"`
	FlowControl string `json:"flowControl"`
}

type serialSession struct {
	app             *App
	info            SerialInfo
	port            serial.Port
	ctx             context.Context
	cancel          context.CancelFunc
	closeOnce       sync.Once
	mu              sync.Mutex
	terminalStarted bool
	txPaused        atomic.Bool
}

func (s *serialSession) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		// Closing the port unblocks the pending Read in the relay, which then
		// observes PortClosed and exits without reporting a device fault.
		_ = s.port.Close()
		s.app.removeSerialSession(s.info.ID)
	})
}

// normalizeSerialProfile canonicalizes a serial connection profile. Serial
// profiles reuse Host for the device name so existing history and search code
// keeps working; network-only fields are cleared.
func normalizeSerialProfile(p *Profile) error {
	p.SerialPort = strings.TrimSpace(p.SerialPort)
	if p.SerialPort == "" {
		return errors.New("请选择或填写串口名称")
	}
	if len(p.SerialPort) > 255 || strings.ContainsAny(p.SerialPort, "\r\n\t") {
		return errors.New("串口名称无效")
	}
	if p.BaudRate == 0 {
		p.BaudRate = 115200
	}
	if p.BaudRate < 110 || p.BaudRate > 4000000 {
		return errors.New("波特率应在 110–4000000 之间")
	}
	if p.DataBits == 0 {
		p.DataBits = 8
	}
	if p.DataBits < 5 || p.DataBits > 8 {
		return errors.New("数据位应为 5–8")
	}
	if p.Parity == "" {
		p.Parity = "none"
	}
	switch p.Parity {
	case "none", "odd", "even", "mark", "space":
	default:
		return errors.New("校验位无效")
	}
	if p.StopBits == 0 {
		p.StopBits = 1
	}
	if p.StopBits != 1 && p.StopBits != 2 {
		return errors.New("停止位应为 1 或 2")
	}
	if p.FlowControl == "" {
		p.FlowControl = "none"
	}
	switch p.FlowControl {
	case "none", "rtscts", "xonxoff":
	default:
		return errors.New("流控方式无效")
	}
	p.Host = p.SerialPort
	p.User = ""
	p.Port = 0
	p.Auth = ""
	p.KeyID = ""
	p.KeyPath = ""
	p.Domain = ""
	p.RDPClipboard = false
	p.ProxyID = ""
	p.Proxy = ProxyConfig{Type: "direct"}
	return nil
}

func serialMode(p Profile) *serial.Mode {
	mode := &serial.Mode{BaudRate: p.BaudRate, DataBits: p.DataBits}
	switch p.Parity {
	case "odd":
		mode.Parity = serial.OddParity
	case "even":
		mode.Parity = serial.EvenParity
	case "mark":
		mode.Parity = serial.MarkParity
	case "space":
		mode.Parity = serial.SpaceParity
	default:
		mode.Parity = serial.NoParity
	}
	if p.StopBits == 2 {
		mode.StopBits = serial.TwoStopBits
	} else {
		mode.StopBits = serial.OneStopBit
	}
	return mode
}

func serialOpenError(portName string, baud int, err error) error {
	var pe serial.PortError
	if errors.As(err, &pe) {
		switch pe.Code() {
		case serial.PortBusy:
			return fmt.Errorf("串口 %s 正被其他程序占用，请关闭占用该串口的程序后重试", portName)
		case serial.PortNotFound:
			return fmt.Errorf("串口 %s 不存在或已被拔出，请刷新串口列表", portName)
		case serial.PermissionDenied:
			return fmt.Errorf("没有权限打开串口 %s；Linux 下请将当前用户加入 dialout 组或检查设备权限", portName)
		case serial.InvalidSpeed:
			return fmt.Errorf("波特率 %d 不被串口 %s 支持", baud, portName)
		case serial.InvalidDataBits, serial.InvalidParity, serial.InvalidStopBits:
			return fmt.Errorf("串口参数（数据位/校验位/停止位）不被串口 %s 支持", portName)
		case serial.InvalidSerialPort:
			return fmt.Errorf("%s 不是可用的串口设备", portName)
		}
	}
	return fmt.Errorf("打开串口 %s 失败：%w", portName, err)
}

func (a *App) serialSessionByID(id string) (*serialSession, error) {
	a.serial.mu.Lock()
	defer a.serial.mu.Unlock()
	s := a.serial.sessions[id]
	if s == nil {
		return nil, errors.New("串口会话不存在，请重新连接")
	}
	select {
	case <-s.ctx.Done():
		return nil, errors.New("串口已关闭，请重新连接")
	default:
		return s, nil
	}
}

func (a *App) removeSerialSession(id string) {
	a.serial.mu.Lock()
	delete(a.serial.sessions, id)
	a.serial.mu.Unlock()
}

func (a *App) startSerial(profileID string) (SerialInfo, error) {
	if err := a.RequireUnlocked(); err != nil {
		return SerialInfo{}, err
	}
	p, err := a.store.Get(profileID)
	if err != nil {
		return SerialInfo{}, err
	}
	if p.Protocol != "serial" {
		return SerialInfo{}, errors.New("请选择串口连接")
	}
	if err := normalizeSerialProfile(&p); err != nil {
		return SerialInfo{}, err
	}
	port, err := serial.Open(p.SerialPort, serialMode(p))
	if err != nil {
		return SerialInfo{}, serialOpenError(p.SerialPort, p.BaudRate, err)
	}
	ctx, cancel := context.WithCancel(a.ctx)
	s := &serialSession{app: a, port: port, ctx: ctx, cancel: cancel, info: SerialInfo{
		ID: randomID(), ProfileID: p.ID, Name: p.Name, SerialPort: p.SerialPort,
		BaudRate: p.BaudRate, DataBits: p.DataBits, Parity: p.Parity,
		StopBits: p.StopBits, FlowControl: p.FlowControl,
	}}
	a.serial.mu.Lock()
	if a.serial.closed || a.ctx.Err() != nil {
		a.serial.mu.Unlock()
		cancel()
		_ = port.Close()
		return SerialInfo{}, errors.New("应用正在退出")
	}
	active := 0
	for _, existing := range a.serial.sessions {
		if existing.ctx.Err() == nil {
			active++
		}
	}
	if active >= serialMaxSessions {
		a.serial.mu.Unlock()
		cancel()
		_ = port.Close()
		return SerialInfo{}, fmt.Errorf("最多同时打开 %d 个串口会话", serialMaxSessions)
	}
	if a.serial.sessions == nil {
		a.serial.sessions = map[string]*serialSession{}
	}
	a.serial.sessions[s.info.ID] = s
	a.serial.mu.Unlock()
	if err := a.store.MarkConnected(p.ID, time.Now().UTC()); err != nil {
		s.Close()
		return SerialInfo{}, fmt.Errorf("无法保存成功连接记录：%w", err)
	}
	// A created session whose terminal never attaches (closed tab, failed
	// renderer) must not keep the physical port reserved forever.
	time.AfterFunc(45*time.Second, func() {
		s.mu.Lock()
		started := s.terminalStarted
		s.mu.Unlock()
		if !started {
			s.Close()
		}
	})
	return s.info, nil
}

func (a *App) serialSessions() []SerialInfo {
	a.serial.mu.Lock()
	defer a.serial.mu.Unlock()
	items := []SerialInfo{}
	for _, s := range a.serial.sessions {
		items = append(items, s.info)
	}
	return items
}

func (a *App) stopSerialProfile(id string) {
	a.serial.mu.Lock()
	closing := []*serialSession{}
	for _, s := range a.serial.sessions {
		if s.info.ProfileID == id {
			closing = append(closing, s)
		}
	}
	a.serial.mu.Unlock()
	for _, s := range closing {
		s.Close()
	}
}

func (a *App) stopSerialLocked() {
	a.serial.mu.Lock()
	closing := []*serialSession{}
	for _, s := range a.serial.sessions {
		closing = append(closing, s)
	}
	a.serial.mu.Unlock()
	for _, s := range closing {
		s.Close()
	}
}

func (a *App) closeSerial() {
	a.serial.mu.Lock()
	a.serial.closed = true
	closing := []*serialSession{}
	for _, s := range a.serial.sessions {
		closing = append(closing, s)
	}
	a.serial.mu.Unlock()
	for _, s := range closing {
		s.Close()
	}
}

// serialTerminal relays the raw serial stream into the same xterm.js WebSocket
// protocol used by SSH terminals. There is no PTY: resize messages are
// ignored, and flow control is applied by the relay because the underlying
// library configures ports without driver-level handshaking.
func (a *App) serialTerminal(w http.ResponseWriter, r *http.Request) {
	s, err := a.serialSessionByID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	s.mu.Lock()
	if s.terminalStarted {
		s.mu.Unlock()
		writeError(w, 409, errors.New("此串口会话已有终端"))
		return
	}
	s.terminalStarted = true
	s.mu.Unlock()
	upgrader := websocket.Upgrader{CheckOrigin: a.allowedOrigin, HandshakeTimeout: 5 * time.Second, ReadBufferSize: 4096, WriteBufferSize: 32768}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.mu.Lock()
		s.terminalStarted = false
		s.mu.Unlock()
		return
	}
	ws.SetReadLimit(2 << 20)
	s.runTerminal(&terminalPeer{ws: ws})
}

const (
	serialXON = 0x11
	serialXOFF = 0x13
)

func (s *serialSession) runTerminal(peer *terminalPeer) {
	// Closing the terminal always releases the physical port; the tab and the
	// port share one lifetime.
	defer s.Close()
	write := func(kind int, data []byte) error {
		peer.ws.SetWriteDeadline(time.Now().Add(15 * time.Second))
		return peer.ws.WriteMessage(kind, data)
	}
	status := func(kind, message string) error {
		data, _ := json.Marshal(map[string]string{"type": kind, "message": message})
		return write(websocket.TextMessage, data)
	}
	stopWatch := context.AfterFunc(s.ctx, func() { peer.ws.Close() })
	defer stopWatch()
	if err := status("ready", ""); err != nil {
		return
	}
	output := make(chan []byte, serialRelayFrames)
	inputs := make(chan string, 64)
	controls := make(chan relayMessage, 64)
	peerClosed := make(chan struct{})
	readErr := make(chan error, 1)
	go func() {
		defer close(peerClosed)
		for {
			var m relayMessage
			if err := peer.ws.ReadJSON(&m); err != nil {
				return
			}
			select {
			case controls <- m:
			case <-s.ctx.Done():
				return
			}
		}
	}()
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := s.port.Read(buffer)
			if n > 0 {
				frame := append([]byte(nil), buffer[:n]...)
				select {
				case output <- frame:
				case <-s.ctx.Done():
					return
				}
			}
			if err != nil {
				select {
				case readErr <- err:
				case <-s.ctx.Done():
				}
				return
			}
		}
	}()
	go func() {
		for {
			select {
			case data := <-inputs:
				if s.app.RequireUnlocked() != nil {
					continue
				}
				// Flow control pauses the writer, never the reader: the device
				// keeps speaking while our own transmit is gated.
				for s.txPaused.Load() {
					select {
					case <-s.ctx.Done():
						return
					case <-time.After(20 * time.Millisecond):
					}
				}
				if _, err := s.port.Write([]byte(data)); err != nil {
					s.cancel()
					return
				}
			case <-s.ctx.Done():
				return
			}
		}
	}()
	if s.info.FlowControl == "rtscts" {
		// The library opens ports without driver CTS gating, so the relay
		// emulates hardware flow control by polling the modem status bits.
		go func() {
			ticker := time.NewTicker(25 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					bits, err := s.port.GetModemStatusBits()
					if err == nil {
						s.txPaused.Store(!bits.CTS)
					}
				case <-s.ctx.Done():
					return
				}
			}
		}()
	}
	credits := 0
	xonxoff := s.info.FlowControl == "xonxoff"
	rxPaused := false
	for {
		var outputCh chan []byte
		if credits < serialRelayFrames {
			outputCh = output
		}
		select {
		case <-s.ctx.Done():
			return
		case <-peerClosed:
			return
		case err := <-readErr:
			if s.ctx.Err() == nil {
				_ = status("exit", "串口读取中断，设备可能已拔出："+err.Error())
			}
			return
		case frame := <-outputCh:
			if write(websocket.BinaryMessage, frame) != nil {
				return
			}
			credits++
			if xonxoff {
				// The device's XON/XOFF characters gate our transmit path.
				for _, b := range frame {
					switch b {
					case serialXON:
						s.txPaused.Store(false)
					case serialXOFF:
						s.txPaused.Store(true)
					}
				}
				// And our receive backpressure tells the device to pause.
				if !rxPaused && credits >= serialRelayFrames {
					rxPaused = true
					_, _ = s.port.Write([]byte{serialXOFF})
				}
			}
		case m := <-controls:
			switch m.Type {
			case "ack":
				if credits > 0 {
					credits--
				}
				if xonxoff && rxPaused && credits <= serialRelayFrames/2 {
					rxPaused = false
					_, _ = s.port.Write([]byte{serialXON})
				}
			case "input":
				if s.app.RequireUnlocked() != nil {
					continue
				}
				if len(m.Data) > 65536 {
					_ = status("error", "输入过多，请分批粘贴")
					return
				}
				select {
				case inputs <- m.Data:
				default:
					_ = status("error", "输入过多，请分批粘贴")
					return
				}
			case "resize":
				// Serial has no PTY geometry; the renderer size is local only.
			}
		}
	}
}

func (a *App) registerSerialHTTP(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/serial/ports", func(w http.ResponseWriter, r *http.Request) {
		ports, err := serial.GetPortsList()
		if err != nil {
			writeError(w, 400, fmt.Errorf("枚举串口失败：%w", err))
			return
		}
		list := []string{}
		list = append(list, ports...)
		writeJSON(w, map[string]any{"ports": list, "sessions": a.serialSessions()})
	})
	mux.HandleFunc("POST /api/serial/sessions", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProfileID string `json:"profileId"`
		}
		if !decode(w, r, &input) {
			return
		}
		info, err := a.startSerial(input.ProfileID)
		respond(w, info, err)
	})
	mux.HandleFunc("GET /api/serial/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		s, err := a.serialSessionByID(r.PathValue("id"))
		if err != nil {
			respond(w, nil, err)
			return
		}
		writeJSON(w, s.info)
	})
	mux.HandleFunc("DELETE /api/serial/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if s, err := a.serialSessionByID(r.PathValue("id")); err == nil {
			s.Close()
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/serial/sessions/{id}/terminal", a.serialTerminal)
}
