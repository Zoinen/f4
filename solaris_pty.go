//go:build !windows

package main

import (
	"os"
	"os/exec"
	"runtime"
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
	defer runtime.KeepAlive(p)
	return p.Cmd.Wait()
}

func (p *SolarisPTY) Run(name string, args ...string) error {
	p.Cmd = exec.Command(name, args...)
	p.Cmd.Stdin = p.Slave
	p.Cmd.Stdout = p.Slave
	p.Cmd.Stderr = p.Slave
	p.Cmd.Env = terminalChildEnv()
	p.Cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true, // Создаем новую сессию для управления TTY
		Setctty: true, // Тот же терминал становится управляющим для сессии
	}

	p.SetSize(80, 24)

	err := p.Cmd.Start()
	if err == nil {
		_ = p.Slave.Close()
		p.Slave = nil
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

	// 2. grantpt: узел подчиненного устройства создается как root:sys с
	// правами 0620, и до передачи его во владение пользователю open()
	// ниже вернет EACCES. Проверка прав в VFS срабатывает раньше, чем
	// ptsopen(), поэтому этот шаг идет первым.
	if err := api.GrantPt(master); err != nil {
		master.Close()
		return nil, err
	}

	// 3. unlockpt: открытие мастера выставляет PTLOCK, и ptsopen()
	// отказывает с EAGAIN, пока блокировка не снята. Второй барьер,
	// независимый от первого: снять нужно оба.
	if err := api.UnlockPt(master); err != nil {
		master.Close()
		return nil, err
	}

	// 4. Получение имени подчиненного (fstat/minor-номер под капотом)
	slaveName, err := api.GetPtsName(master)
	if err != nil {
		master.Close()
		return nil, err
	}

	// 5. Открытие подчиненного терминала. O_NOCTTY — тот же флаг, что уже
	// стоит при открытии слейва в linux/freebsd/darwin-ветках: без него
	// открытие слейва в процессе без управляющего терминала само делает
	// его управляющим, и им рискует стать терминал самого f4, а не только
	// терминал дочернего шелла.
	slave, err := api.Open(slaveName, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, err
	}

	// 6. Построение стека модулей STREAMS на слейве (LIFO порядок вызовов I_PUSH)
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
