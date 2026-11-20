// Command ttx-gen generates DVB teletext subtitles for testing: one subtitle
// page per language, refreshed every period, showing a sample sentence and a
// time stamp. It writes a teletext-only stream, or inserts the teletext into
// an existing transport stream with video and audio.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	teletext "github.com/Eyevinn/go-teletext"
	"github.com/Eyevinn/go-teletext/internal"
	"github.com/Eyevinn/go-teletext/mpegts"
)

const (
	appName = "ttx-gen"
	tick    = 40 * time.Millisecond // one 25 Hz frame
)

var usg = `%s generates DVB teletext subtitles (EN 300 472), one page per language.
Each cue shows a sample sentence and a time stamp, for example
"Svenska 12:34:56.080 (42)". The page header is sent in one PES and the rows in
the next, 40 ms later; the time stamp is the PTS of the header PES.

Without -i it writes a teletext-only stream whose PCR and PTS are the UTC time of
day. With -i it inserts the teletext into an existing transport stream, with PTS
that follow the input's PCR, and prints the wall-clock time of the PTS
(-clock wall) or the PTS itself (-clock pts).

Usage of %s:
`

type options struct {
	out, udp        string
	input           string
	live            bool
	duration        time.Duration
	langs, pages    string
	period, cue     time.Duration
	level15         bool
	designation     string
	singleHeight    bool
	ptsOffset       time.Duration
	heartbeat       time.Duration
	heartbeatKind   string
	cuesOn, cuesOff time.Duration
	start           string
	clock           string
	list            bool
	pid             uint
	program         uint
	replaceNull     bool
	muxRate         uint64
	hearingImpaired bool
	version         bool
}

func parseOptions(fs *flag.FlagSet, args []string) (*options, error) {
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, usg, appName, appName)
		fmt.Fprintf(os.Stderr, "%s [options]\n\noptions:\n", appName)
		fs.PrintDefaults()
	}
	o := options{}
	fs.StringVar(&o.out, "o", "ttx-gen.ts", "output file, '-' for stdout")
	fs.StringVar(&o.udp, "udp", "", "send to UDP host:port instead of -o (implies -live)")
	fs.StringVar(&o.input, "i", "",
		"insert into this transport stream: a file, '-' for stdin, or udp://host:port to listen on")
	fs.BoolVar(&o.live, "live", false, "pace output to the wall clock (with -i: a file input is paced by its PCR)")
	fs.DurationVar(&o.duration, "duration", 60*time.Second,
		"length of the stream, 0 = forever (live only); with -i the default is the whole input")
	fs.StringVar(&o.langs, "langs", "swe,nor,dan,fin,ger,fre,pol,rus,gre",
		"language presets, comma-separated (see -list)")
	fs.StringVar(&o.pages, "pages", "",
		"comma-separated page numbers in language order, default 888, 788, ..., 188, 889, 789, ...")
	fs.DurationVar(&o.period, "period", 2*time.Second, "time between subtitle cues")
	fs.DurationVar(&o.cue, "cue", 1600*time.Millisecond, "time a cue stays on screen")
	fs.BoolVar(&o.level15, "level15", true, "send X/26 enhancements for characters outside G0")
	fs.StringVar(&o.designation, "designation", "auto", "charset designation: auto, x28, m29 or none")
	fs.BoolVar(&o.singleHeight, "single-height", false, "single instead of double height rows")
	fs.DurationVar(&o.ptsOffset, "pts-offset", 40*time.Millisecond,
		"PTS minus PCR at transmission (EN 300 472 allows at most 40 ms retention)")
	fs.DurationVar(&o.heartbeat, "heartbeat", tick,
		"send a heartbeat PES when no other teletext was sent for this long, 0 = off")
	fs.StringVar(&o.heartbeatKind, "heartbeat-kind", "filler",
		"heartbeat content: filler (page header mFF per magazine) or stuffing (stuffing data units only)")
	fs.DurationVar(&o.cuesOn, "cues-on", 0, "send cues for this long, then pause for -cues-off; 0 = always")
	fs.DurationVar(&o.cuesOff, "cues-off", 0, "pause without cues (heartbeats continue) after each -cues-on")
	fs.StringVar(&o.start, "start", "",
		"start time RFC 3339, default now; makes file output reproducible")
	fs.StringVar(&o.clock, "clock", "wall", "with -i, time stamp in cues: wall (wall-clock time of the PTS) or pts")
	fs.BoolVar(&o.list, "list", false, "list language presets and exit")
	fs.UintVar(&o.pid, "pid", 0,
		"teletext PID; 0 = 0x101, or with -i the PID after the highest PID of the program's streams")
	fs.UintVar(&o.program, "program", 0, "with -i, program number to add teletext to, 0 = first in the PAT")
	fs.BoolVar(&o.replaceNull, "replace-null", true,
		"with -i, drop a null packet for each inserted packet when the input has them")
	fs.Uint64Var(&o.muxRate, "muxrate", 0,
		"with -i, write constant bitrate output at this rate (bit/s): restamps PCR and stuffs with null packets")
	fs.BoolVar(&o.hearingImpaired, "hearing-impaired", false,
		"signal pages as subtitles for the hard of hearing (type 0x05)")
	fs.BoolVar(&o.version, "version", false, fmt.Sprintf("Get %s version", appName))
	err := fs.Parse(args[1:])
	if err == nil && o.input != "" {
		durationSet := false
		fs.Visit(func(f *flag.Flag) { durationSet = durationSet || f.Name == "duration" })
		if !durationSet {
			o.duration = 0
		}
	}
	return &o, err
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	o, err := parseOptions(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if o.version {
		fmt.Fprintf(stdout, "%s %s\n", appName, internal.Version())
		return nil
	}
	if o.list {
		for _, l := range teletext.Languages {
			fmt.Fprintf(stdout, "%-4s %-12s %-34s %s\n", l.Code, l.Name, l.Charset.Name, l.Note)
		}
		return nil
	}
	s, err := newSchedule(o)
	if err != nil {
		return err
	}
	if o.clock != "wall" && o.clock != "pts" {
		return fmt.Errorf("unknown -clock %q", o.clock)
	}
	if o.udp != "" {
		o.live = true
	}
	if o.input == "" && !o.live && o.duration <= 0 {
		return errors.New("-duration must be > 0 for file output")
	}
	if _, err := startTime(o); err != nil {
		return err
	}

	var (
		r        io.Reader
		paceFile bool
	)
	if o.input != "" {
		var closeIn func()
		if r, closeIn, paceFile, err = openInput(o.input); err != nil {
			return err
		}
		defer closeIn()
	}

	w, flush, closeOut, err := output(o, stdout)
	if err != nil {
		return err
	}
	defer closeOut()

	if o.input != "" {
		return insert(o, s, r, paceFile && o.live, w, flush)
	}
	return generate(o, s, w, flush)
}

// startTime returns -start, or now.
func startTime(o *options) (time.Time, error) {
	if o.start == "" {
		return time.Now().UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, o.start)
	if err != nil {
		return time.Time{}, fmt.Errorf("-start: %w", err)
	}
	return t.UTC(), nil
}

// output returns the writer for the stream, a flush function and a close function.
func output(o *options, stdout io.Writer) (io.Writer, func() error, func(), error) {
	switch {
	case o.udp != "":
		conn, err := net.Dial("udp", o.udp)
		if err != nil {
			return nil, nil, nil, err
		}
		uw := &udpWriter{conn: conn}
		return uw, uw.Flush, func() { _ = conn.Close() }, nil
	case o.out == "-":
		bw := bufio.NewWriter(stdout)
		return bw, bw.Flush, func() {}, nil
	default:
		f, err := os.Create(o.out)
		if err != nil {
			return nil, nil, nil, err
		}
		bw := bufio.NewWriter(f)
		return bw, bw.Flush, func() { _ = f.Close() }, nil
	}
}

// udpWriter sends TS packets in datagrams of up to 7 packets.
type udpWriter struct {
	conn net.Conn
	buf  []byte
}

func (u *udpWriter) Write(b []byte) (int, error) {
	u.buf = append(u.buf, b...)
	for len(u.buf) >= 7*mpegts.PacketSize {
		if _, err := u.conn.Write(u.buf[:7*mpegts.PacketSize]); err != nil {
			return 0, err
		}
		u.buf = u.buf[7*mpegts.PacketSize:]
	}
	return len(b), nil
}

func (u *udpWriter) Flush() error {
	if len(u.buf) == 0 {
		return nil
	}
	_, err := u.conn.Write(u.buf)
	u.buf = u.buf[:0]
	return err
}
