package main

import (
	"math/rand"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func TestArkanoid_Init(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	af := NewArkanoidFrame()
	if af == nil {
		t.Fatal("Failed to create Arkanoid frame")
	}
	t.Cleanup(af.Close)

	if af.lives != 3 {
		t.Errorf("Expected 3 lives, got %d", af.lives)
	}

	if len(af.bricks) == 0 {
		t.Error("Arkanoid started with no bricks")
	}

}

func TestArkanoid_PhysicsAndCollisions(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	af := NewArkanoidFrame()
	af.Close() // Останавливаем фоновый цикл, чтобы избежать конфликтов в тесте
	time.Sleep(10 * time.Millisecond)
	af.rng = rand.New(rand.NewSource(1)) //nolint:gosec // Deterministic randomness only controls the ball's bounce direction in this test.

	height := af.Y2 - af.Y1 - 1

	// 1. Тест отскока от левой стены
	af.ballX = 0.0
	af.ballDX = -1.0
	af.ballY = 5.0
	af.ballDY = 0.0

	af.update()

	if af.ballDX <= 0 {
		t.Errorf("Expected positive ballDX after bouncing off left wall, got %f", af.ballDX)
	}

	// 2. Тест отскока от ракетки
	af.paddleX = 10
	af.paddleW = 8
	af.ballX = 12.0
	af.ballY = float64(height - 1)
	af.ballDX = 0.0
	af.ballDY = 1.0

	af.update()

	if af.ballDY >= 0 {
		t.Errorf("Expected negative ballDY after bouncing off paddle, got %f", af.ballDY)
	}

	// 3. Тест попадания в кирпич и начисления очков
	var targetBrick *brick
	for i := range af.bricks {
		if af.bricks[i].hp > 0 {
			targetBrick = &af.bricks[i]
			break
		}
	}
	if targetBrick == nil {
		t.Fatal("No bricks available for collision test")
	}

	initialHP := targetBrick.hp
	initialScore := af.score

	// Направляем мяч в выбранный кирпич
	af.ballX = float64(targetBrick.x + 1)
	af.ballY = float64(targetBrick.y + 1)
	af.ballDX = 0.0
	af.ballDY = -1.0

	af.update()

	if targetBrick.hp != initialHP-1 {
		t.Errorf("Expected brick HP to be %d, got %d", initialHP-1, targetBrick.hp)
	}
	if af.score <= initialScore {
		t.Error("Score did not increase after hitting a brick")
	}
	if af.combo != 1 {
		t.Errorf("Expected combo 1, got %d", af.combo)
	}
}

func TestArkanoid_AutoplayAI(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	af := NewArkanoidFrame()
	af.Close()
	time.Sleep(10 * time.Millisecond)

	af.autoPlay = true
	af.paddleX = 5
	af.paddleW = 8

	// Смещаем мяч вправо и пускаем вниз
	af.ballX = 30.0
	af.ballY = 5.0
	af.ballDX = 0.5
	af.ballDY = 0.5

	initialPaddleX := af.paddleX
	af.update()

	if af.paddleX <= initialPaddleX {
		t.Errorf("Expected AI to move paddle right (paddleX > %d), got %d", initialPaddleX, af.paddleX)
	}
}

func TestArkanoid_HighScores(t *testing.T) {
	// Подменяем путь к директории настроек, чтобы не мусорить на диске
	tmpDir := t.TempDir()
	oldUserConfigDir := userConfigDir
	userConfigDir = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { userConfigDir = oldUserConfigDir })

	oldHighScores := ArkHighScores
	t.Cleanup(func() { ArkHighScores = oldHighScores })

	// Сбрасываем рекорды в памяти
	ArkHighScores = nil

	// Сохраняем новые рекорды
	ArkHighScores = []ArkScore{
		{Name: "Hero", Score: 5000, Level: 3},
		{Name: "Champ", Score: 3000, Level: 2},
	}
	saveArkScores()

	// Очищаем рекорды в памяти и читаем их заново с диска
	ArkHighScores = nil
	loadArkScores()

	if len(ArkHighScores) != 2 || ArkHighScores[0].Name != "Hero" || ArkHighScores[0].Score != 5000 {
		t.Errorf("Failed to save or load high scores. Loaded list: %+v", ArkHighScores)
	}
}
