// SPDX-License-Identifier: GPL-3.0-or-later

// Package f4hwnkiss models the host side of the KISS TNC overlay app of the
// F4HWN firmware for the Quansheng UV-K1 / UV-K5 V3, as read from its source:
// App/apps/kisstnc/kisstnc_app.c (v6.3.0, unchanged on feature_update_v7),
// App/usb/usbd_cdc_if.c and App/apps/app_overlay.c.
//
// It is a behavioural model for testing AXLink's pacing, not the firmware:
//
//   - USB data lands in a 256-byte ring without flow control. The app reads
//     16 bytes every 10 ms; 256 or more unread bytes flush the whole ring.
//   - The KISS parser holds one frame. Bytes that arrive while a frame waits
//     for the channel are discarded, without a counter on the radio.
//   - Every 50 ms a housekeeping slot sends a waiting frame with
//     p-persistence when the channel is clear.
//   - While a frame is on the air, interrupts are masked: the app reads
//     nothing and USB holds back what the host writes until the end of the
//     transmission, then delivers it at once.
//
// Config.AckMode adds the proposed ACKMODE extension (KISS command 0x0C),
// which the original app does not have.
package f4hwnkiss

import (
	"time"

	"github.com/TechnikWeber/AXLink/internal/airtime"
	"github.com/TechnikWeber/AXLink/internal/kiss"
)

// Values from the firmware source.
const (
	RingSize     = 256                   // VCP_RxBuf
	ReadChunk    = 16                    // kissPoll: bytes per read
	PollEvery    = 10 * time.Millisecond // SERIAL_EVERY
	SlotEvery    = 50 * time.Millisecond // HOUSE_EVERY
	FrameMax     = 330                   // FRAME_MAX, FCS included
	TxDelayFlags = 40                    // TXDELAY_DEF: 267 ms
	TailFlags    = 3
	Persist      = 63 // P_DEF
	Baud         = 1200
	minDataLen   = 16 // a data frame needs 16 bytes with the command byte
)

// Config sets the model's options. The zero value is the original app with
// a channel that is always clear and a persistence draw that always sends.
type Config struct {
	AckMode bool                       // model the proposed ACKMODE extension
	Rand    func() uint32              // the app's rand32(); nil returns 0
	Busy    func(t time.Duration) bool // channel busy (DCD) at t; nil is never busy
}

// Transmission is a frame that went on the air.
type Transmission struct {
	Start, End time.Duration
	Frame      []byte // AX.25 frame without FCS
}

// Loss records unread bytes flushed because the ring was lapped.
type Loss struct {
	At    time.Duration
	Bytes int
}

// Ack is an ACKMODE echo the app sent to the host.
type Ack struct {
	At  time.Duration
	Tag uint16
}

// App is the model. It advances in virtual time; all times are offsets from
// the moment the app started.
type App struct {
	cfg Config
	now time.Duration

	unread []byte // bytes in the USB ring the app has not read
	held   []byte // host data USB holds back while interrupts are masked

	kIn, kEsc, txReady bool
	kLen               int
	tx                 [FrameMax]byte
	txFlags, persist   int

	inTx      bool
	txEnd     time.Duration
	pendAck   bool
	pendTag   uint16
	nextPoll  time.Duration
	nextSlot  time.Duration
	discarded int

	sent   []Transmission
	losses []Loss
	acks   []Ack
}

// New starts the model at time 0.
func New(cfg Config) *App {
	return &App{
		cfg:      cfg,
		kIn:      true, // the app accepts a first frame without a leading FEND
		txFlags:  TxDelayFlags,
		persist:  Persist,
		nextPoll: PollEvery,
		nextSlot: SlotEvery,
	}
}

// Now returns the model's current time.
func (a *App) Now() time.Duration { return a.now }

// Transmitted returns the frames sent on the air so far.
func (a *App) Transmitted() []Transmission { return a.sent }

// Losses returns the ring overruns so far.
func (a *App) Losses() []Loss { return a.losses }

// Acks returns the ACKMODE echoes so far (only with Config.AckMode).
func (a *App) Acks() []Ack { return a.acks }

// Discarded returns how many host bytes the parser threw away, mostly while
// a frame was waiting. The radio shows no counter for this.
func (a *App) Discarded() int { return a.discarded }

// Transmitting reports whether a frame is on the air.
func (a *App) Transmitting() bool { return a.inTx }

// Write delivers bytes the host writes at time at. A time before Now counts
// as Now.
func (a *App) Write(at time.Duration, p []byte) {
	a.Advance(at)
	if a.inTx {
		a.held = append(a.held, p...)
		return
	}
	a.unread = append(a.unread, p...)
}

// Advance runs the model up to time to.
func (a *App) Advance(to time.Duration) {
	for {
		next := a.nextEvent()
		if next > to {
			break
		}
		a.now = next
		a.step()
	}
	if to > a.now {
		a.now = to
	}
}

func (a *App) nextEvent() time.Duration {
	if a.inTx {
		return a.txEnd
	}
	return min(a.nextPoll, a.nextSlot)
}

func (a *App) step() {
	if a.inTx {
		a.endTx()
		return
	}
	// The sampling loop polls USB before it runs a housekeeping slot.
	if a.now >= a.nextPoll {
		a.poll()
		a.nextPoll += PollEvery
	}
	if a.now >= a.nextSlot {
		a.slot()
		a.nextSlot += SlotEvery
	}
}

func (a *App) poll() {
	if len(a.unread) >= RingSize {
		a.losses = append(a.losses, Loss{At: a.now, Bytes: len(a.unread)})
		a.unread = a.unread[:0]
		return
	}
	n := min(ReadChunk, len(a.unread))
	for _, b := range a.unread[:n] {
		a.kissByte(b)
	}
	a.unread = append(a.unread[:0], a.unread[n:]...)
}

// kissByte follows the app's kissByte(), plus the ACKMODE command.
func (a *App) kissByte(b byte) {
	if b == kiss.FEND {
		if a.kIn && a.kLen > 1 {
			cmd, v := kiss.Command(a.tx[0]&0x0F), int(a.tx[1])
			switch {
			case cmd == kiss.CmdData:
				if a.kLen >= minDataLen {
					a.txReady = true
				}
			case cmd == kiss.CmdAckMode && a.cfg.AckMode:
				if a.kLen >= minDataLen+2 {
					a.txReady = true
					a.pendAck = true
					a.pendTag = uint16(a.tx[1])<<8 | uint16(a.tx[2])
				}
			case cmd == kiss.CmdTxDelay:
				a.txFlags = min(max(v+v>>1, 8), 255)
			case cmd == kiss.CmdPersist:
				a.persist = v
			}
		}
		a.kIn = !a.txReady
		a.kEsc = false
		if !a.txReady {
			a.kLen = 0
		}
		return
	}
	if !a.kIn {
		a.discarded++
		return
	}
	if a.kEsc {
		a.kEsc = false
		switch b {
		case kiss.TFEND:
			b = kiss.FEND
		case kiss.TFESC:
			b = kiss.FESC
		default:
			a.kIn = false
			return
		}
	} else if b == kiss.FESC {
		a.kEsc = true
		return
	}
	if a.kLen >= FrameMax-2 { // room for the FCS
		a.kIn = false
		return
	}
	a.tx[a.kLen] = b
	a.kLen++
}

func (a *App) slot() {
	if !a.txReady {
		return
	}
	if a.cfg.Busy != nil && a.cfg.Busy(a.now) {
		return
	}
	var r uint32
	if a.cfg.Rand != nil {
		r = a.cfg.Rand()
	}
	if int(r&255) <= a.persist {
		a.transmit()
	}
}

func (a *App) transmit() {
	start := 1 // after the KISS command byte
	if a.pendAck {
		start = 3 // and the two tag bytes
	}
	frame := append([]byte(nil), a.tx[start:a.kLen]...)
	a.txReady, a.kIn, a.kLen = false, false, 0
	bits := airtime.Bits(frame, a.txFlags, TailFlags)
	a.inTx = true
	a.txEnd = a.now + airtime.Duration(bits, Baud)
	a.sent = append(a.sent, Transmission{Start: a.now, End: a.txEnd, Frame: frame})
}

func (a *App) endTx() {
	a.inTx = false
	if a.pendAck {
		a.acks = append(a.acks, Ack{At: a.now, Tag: a.pendTag})
		a.pendAck = false
	}
	// USB delivers what it held back, all at once.
	a.unread = append(a.unread, a.held...)
	a.held = a.held[:0]
	// The sampling loop resumes: next poll in 10 ms, next slot in 50 ms.
	a.nextPoll = a.now + PollEvery
	a.nextSlot = a.now + SlotEvery
}
