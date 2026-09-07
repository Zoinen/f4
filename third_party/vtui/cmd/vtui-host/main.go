package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func main() {
	protoFDFlag := flag.String("protocol-fd", "", "File descriptor number for bidirectional protocol communication")
	protoInFDFlag := flag.String("protocol-in-fd", "", "File descriptor number for protocol input")
	protoOutFDFlag := flag.String("protocol-out-fd", "", "File descriptor number for protocol output")
	socketFlag := flag.String("socket", "", "Unix domain socket path for protocol communication")
	socketListenFlag := flag.String("socket-listen", "", "Unix domain socket path to listen on for protocol communication")
	backendFlag := flag.String("backend", "", "Rendering backend (ansi, winapi, gogpu, x11, wayland, ebiten)")
	flag.Parse()

	if *protoInFDFlag != "" && *protoOutFDFlag != "" {
		inFD, err1 := strconv.Atoi(*protoInFDFlag)
		outFD, err2 := strconv.Atoi(*protoOutFDFlag)
		if err1 == nil && err2 == nil {
			inF := os.NewFile(uintptr(inFD), "protocol_in")
			outF := os.NewFile(uintptr(outFD), "protocol_out")
			defer inF.Close()
			defer outF.Close()
			runSession(inF, outF, *backendFlag)
			return
		}
	}

	if *protoFDFlag != "" {
		fd, err := strconv.Atoi(*protoFDFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vtui-host: invalid --protocol-fd: %v\n", err)
			os.Exit(1)
		}
		f := os.NewFile(uintptr(fd), "protocol_stream")
		defer f.Close()
		runSession(f, f, *backendFlag)
		return
	}

	if *socketListenFlag != "" {
		_ = os.Remove(*socketListenFlag)
		l, err := net.Listen("unix", *socketListenFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vtui-host: failed to listen on socket %s: %v\n", *socketListenFlag, err)
			os.Exit(1)
		}
		defer l.Close()
		defer os.Remove(*socketListenFlag)
		conn, err := l.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "vtui-host: failed to accept socket connection: %v\n", err)
			os.Exit(1)
		}
		defer conn.Close()
		runSession(conn, conn, *backendFlag)
		return
	}

	if *socketFlag != "" {
		conn, err := net.Dial("unix", *socketFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vtui-host: failed to connect to socket %s: %v\n", *socketFlag, err)
			os.Exit(1)
		}
		defer conn.Close()
		runSession(conn, conn, *backendFlag)
		return
	}

	// Default to stdin/stdout
	runSession(os.Stdin, os.Stdout, *backendFlag)
}

func runSession(in io.Reader, out io.Writer, backend string) {
	// Guard against parent process exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigChan
		vtui.FrameManager.Shutdown()
		os.Exit(0)
	}()

	w, h, err := vtui.GetTerminalSize()
	if err != nil || w <= 0 || h <= 0 {
		w, h = 80, 25
	}

	scr := vtui.NewScreenBuf()
	if backend == "" {
		backend = vtui.DefaultConsoleBackend()
	}
	if backend == "winapi" || backend == "win32" {
		scr.Renderer = vtui.NewWin32ConsoleRenderer(scr)
	}
	scr.AllocBuf(w, h)
	vtui.FrameManager.Init(scr)
	vtui.FrameManager.Push(vtui.NewDesktop())
	vtui.FrameManager.SetHostMode(true)

	session := vtui.NewProtocolSession(in, out, vtui.FrameManager)

	go func() {
		_ = session.Serve()
		vtui.FrameManager.Shutdown()
	}()

	if backend != "ansi" && backend != "winapi" && backend != "win32" && backend != "" {
		_ = vtui.RunInGUIWindow(w, h, backend, "", 18.0, func() {})
	} else {
		restore, err := vtinput.Enable()
		if err == nil && restore != nil {
			defer restore()
		}
		reader := vtinput.NewReader(os.Stdin, false)
		vtui.FrameManager.Run(reader)
	}
}
