//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// SolarisPTY представляет собой выделенную пару псевдотерминала Solaris/Illumos,
// полностью реализующую интерфейс PtyBackend приложения f4.
type SolarisPTY struct {
	Master    *os.File
	Slave     *os.File
	Name      string
	Cmd       *exec.Cmd
	shellPgrp int
	api       SolarisStreamsAPI
}

func (p *SolarisPTY) Read(buf []byte) (int, error) {
	return p.Master.Read(buf)
}

func (p *SolarisPTY) Write(buf []byte) (int, error) {
	return p.Master.Write(buf)
}

func (p *SolarisPTY) Close() error {
	if p.Cmd != nil && p.Cmd.Process != nil {
		// Убиваем всю группу процессов (используя отрицательный PID)
		_ = unix.Kill(-p.Cmd.Process.Pid, unix.SIGKILL)
		p.Cmd.Process.Kill()
	}
	var err1, err2 error
	if p.Slave != nil {
		err1 = p.Slave.Close()
	}
	if p.Master != nil {
		err2 = p.Master.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

func (p *SolarisPTY) SetSize(cols, rows int) {
	_ = p.api.SetSize(p.Master, cols, rows)
}

func (p *SolarisPTY) Wait() error {
	if p.Cmd == nil {
		return nil
	}
	return p.Cmd.Wait()
}

func (p *SolarisPTY) Run(name string, args ...string) error {
	p.Cmd = exec.Command(name, args...)
	p.Cmd.Stdin = p.Slave
	p.Cmd.Stdout = p.Slave
	p.Cmd.Stderr = p.Slave
	p.Cmd.Env = terminalChildEnv()
	p.Cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Создаем новую сессию для управления TTY
	}

	p.SetSize(80, 24)

	err := p.Cmd.Start()
	if err == nil {
		p.shellPgrp, _ = unix.Getpgid(p.Cmd.Process.Pid)
	}
	return err
}

func (p *SolarisPTY) IsBusy() bool {
	return p.api.IsBusy(p.Master, p.shellPgrp)
}

// OpenSolarisPTY реализует чистый алгоритм выделения PTY для Solaris/Illumos.
// Он открывает мастер-клон, определяет подчиненное имя (ptsname) и настраивает STREAMS.
func OpenSolarisPTY(api SolarisStreamsAPI) (*SolarisPTY, error) {
	// 1. Открытие мультиплексора мастера
	master, err := api.Open("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	// 2. Получение имени подчиненного (fstat/minor-номер под капотом)
	slaveName, err := api.GetPtsName(master)
	if err != nil {
		master.Close()
		return nil, err
	}

	// 3. Открытие подчиненного терминала
	slave, err := api.Open(slaveName, os.O_RDWR, 0)
	if err != nil {
		master.Close()
		return nil, err
	}

	// 4. Построение стека модулей STREAMS на слейве (LIFO порядок вызовов I_PUSH)
	// 'ptem' эмулирует аппаратный терминал
	if err := api.IoctlPush(slave, "ptem"); err != nil {
		slave.Close()
		master.Close()
		return nil, err
	}

	// 'ldterm' обеспечивает обработку строк (эрэсинг, канонический ввод)
	if err := api.IoctlPush(slave, "ldterm"); err != nil {
		slave.Close()
		master.Close()
		return nil, err
	}

	// 'ttcompat' обеспечивает совместимость с классическими BSD/V7 ioctl
	if err := api.IoctlPush(slave, "ttcompat"); err != nil {
		slave.Close()
		master.Close()
		return nil, err
	}

	return &SolarisPTY{
		Master: master,
		Slave:  slave,
		Name:   slaveName,
		api:    api,
	}, nil
}
