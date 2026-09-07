<?php

namespace Vtui;

class VtuiError extends \Exception {
    public ?int $replyTo;

    public function __construct(string $code, string $message, ?int $replyTo = null) {
        parent::__construct("[{$code}] {$message}");
        $this->code = $code;
        $this->replyTo = $replyTo;
    }
}

class Session {
    private string $hostBin;
    private string $backend;
    private int $seq = 0;
    private $proc = null;
    private $socket = null;

    public function __construct(array $options = []) {
        $this->hostBin = $options['hostBin'] ?? $this->findHostBinary();
        $this->backend = $options['backend'] ?? getenv('VTUI_BACKEND') ?: 'ansi';
    }

    private function findHostBinary(): string {
        if ($env = getenv('VTUI_HOST_BIN')) {
            return $env;
        }

        $base = realpath(__DIR__ . '/../../..');
        $candidates = [
            $base . '/cmd/vtui-host/vtui-host',
            $base . '/bindings/cpp/build/vtui-host',
            $base . '/bindings/c/build/vtui-host',
            $base . '/bindings/build/vtui-host',
            $base . '/build/vtui-host',
            $base . '/vtui-host',
            getenv('HOME') . '/go/bin/vtui-host',
        ];

        foreach ($candidates as $cand) {
            if ($cand && file_exists($cand) && is_executable($cand)) {
                return $cand;
            }
        }

        // Try building via go if present
        if ($base && is_dir($base . '/cmd/vtui-host')) {
            $target = $base . '/vtui-host';
            @exec("go build -o " . escapeshellarg($target) . " ./cmd/vtui-host 2>/dev/null");
            if (file_exists($target) && is_executable($target)) {
                return $target;
            }
        }

        return 'vtui-host';
    }

    public function start(): void {
        $pair = @stream_socket_pair(STREAM_PF_UNIX, STREAM_SOCK_STREAM, STREAM_IPPROTO_IP);
        if ($pair === false) {
            throw new \RuntimeException("Failed to create stream_socket_pair for IPC");
        }

        $descriptors = [
            0 => STDIN,
            1 => STDOUT,
            2 => STDERR,
            3 => $pair[1],
        ];

        $cmd = escapeshellcmd($this->hostBin) . " --protocol-fd=3 --backend=" . escapeshellarg($this->backend);
        $this->proc = proc_open($cmd, $descriptors, $pipes, null, $_ENV);
        fclose($pair[1]);

        if (!is_resource($this->proc)) {
            fclose($pair[0]);
            throw new \RuntimeException("Failed to spawn vtui-host process: {$cmd}");
        }

        $this->socket = $pair[0];
        stream_set_blocking($this->socket, true);

        // Handshake
        $this->send(['op' => 'hello', 'version' => 1]);
        $resp = $this->recv();
        if (!$resp || ($resp['op'] ?? '') === 'error') {
            $this->close();
            throw new VtuiError($resp['code'] ?? 'HANDSHAKE_FAILED', $resp['message'] ?? 'Handshake failed');
        }
    }

    public function send(array $msg): int {
        $this->seq++;
        if (!isset($msg['seq'])) {
            $msg['seq'] = $this->seq;
        }
        $line = json_encode($msg, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE) . "\n";
        if (is_resource($this->socket)) {
            fwrite($this->socket, $line);
            fflush($this->socket);
        }
        return $this->seq;
    }

    public function recv(?float $timeout = null): ?array {
        if (!is_resource($this->socket)) {
            return null;
        }

        if ($timeout !== null) {
            $r = [$this->socket];
            $w = null;
            $e = null;
            $sec = (int)$timeout;
            $usec = (int)(($timeout - $sec) * 1000000);
            $num = @stream_select($r, $w, $e, $sec, $usec);
            if ($num === false || $num === 0) {
                return null;
            }
        }

        $line = fgets($this->socket);
        if ($line === false || $line === '') {
            return null;
        }

        $msg = json_decode(trim($line), true);
        if (isset($msg['op']) && $msg['op'] === 'error') {
            throw new VtuiError($msg['code'] ?? 'ERROR', $msg['message'] ?? '', $msg['replyTo'] ?? null);
        }
        return $msg;
    }

    public function mount(string $frameId, array $tree): void {
        $this->send(['op' => 'mount', 'frameId' => $frameId, 'tree' => $tree]);
    }

    public function patch(string $frameId, array $ops): void {
        $this->send(['op' => 'patch', 'frameId' => $frameId, 'ops' => $ops]);
    }

    public function message(string $title, string $text, array $buttons = ['&Ok']): void {
        $this->send(['op' => 'message', 'title' => $title, 'text' => $text, 'buttons' => $buttons]);
    }

    public function quit(): void {
        try {
            $this->send(['op' => 'quit']);
        } catch (\Throwable $e) {}
    }

    public function close(): void {
        $this->quit();
        if (is_resource($this->socket)) {
            @fclose($this->socket);
            $this->socket = null;
        }
        if (is_resource($this->proc)) {
            @proc_close($this->proc);
            $this->proc = null;
        }
    }
}

class Ui {
    private Session $session;
    private array $containerStack = [];
    private array $values = [];
    private array $clickedIds = [];
    private bool $mounted = false;
    public string $rootId = "mainDlg";
    private ?array $currentRoot = null;

    public function __construct(Session $session) {
        $this->session = $session;
    }

    public function dialog(string $title, int $w = 40, ?callable $callback = null): ?array {
        $node = [
            'type' => 'Dialog',
            'id' => $this->rootId,
            'props' => ['title' => $title, 'autoSize' => true, 'center' => true],
            'layout' => ['type' => 'VBox', 'spacing' => 1, 'margins' => [1, 2, 1, 2]],
            'children' => [],
        ];
        $this->containerStack[] = &$node;

        if ($callback) {
            $callback();
        }

        array_pop($this->containerStack);
        $this->currentRoot = $node;
        return $this->currentRoot;
    }

    public function edit(string $label, string $default = "", ?string $id = null): string {
        $editId = $id ?? 'edit_' . trim(str_replace('&', '', $label));
        if (!array_key_exists($editId, $this->values)) {
            $this->values[$editId] = $default;
        }

        $groupNode = [
            'type' => 'Group',
            'layout' => ['type' => 'Form', 'spacing' => 1],
            'children' => [
                ['type' => 'Label', 'props' => ['text' => $label, 'buddy' => $editId]],
                ['type' => 'Edit', 'id' => $editId, 'props' => ['text' => $this->values[$editId]]],
            ],
        ];

        if (!empty($this->containerStack)) {
            $this->containerStack[count($this->containerStack) - 1]['children'][] = $groupNode;
        }

        return $this->values[$editId];
    }

    public function button(string $text, ?string $id = null): bool {
        $btnId = $id ?? 'btn_' . trim(str_replace('&', '', $text));
        $cmdId = 1000 + abs(crc32($btnId)) % 8000;

        $node = [
            'type' => 'Button',
            'id' => $btnId,
            'props' => ['text' => $text, 'command' => $cmdId],
        ];

        if (!empty($this->containerStack)) {
            $this->containerStack[count($this->containerStack) - 1]['children'][] = $node;
        }

        if (isset($this->clickedIds[$btnId])) {
            unset($this->clickedIds[$btnId]);
            return true;
        }
        return false;
    }

    public function checkbox(string $text, bool $default = false, ?string $id = null): bool {
        $chkId = $id ?? 'chk_' . trim(str_replace('&', '', $text));
        if (!array_key_exists($chkId, $this->values)) {
            $this->values[$chkId] = $default;
        }

        $node = [
            'type' => 'Checkbox',
            'id' => $chkId,
            'props' => ['text' => $text, 'state' => $this->values[$chkId] ? 1 : 0],
        ];

        if (!empty($this->containerStack)) {
            $this->containerStack[count($this->containerStack) - 1]['children'][] = $node;
        }

        return (bool)$this->values[$chkId];
    }

    public function message(string $title, string $text, array $buttons = ['&Ok']): void {
        $this->session->message($title, $text, $buttons);
    }

    public function _sync(): void {
        if (!$this->currentRoot) {
            return;
        }
        if (!$this->mounted) {
            $this->session->mount($this->rootId, $this->currentRoot);
            $this->mounted = true;
        }
        $this->clickedIds = [];
    }

    public function _processEvent(?array $ev): void {
        if (!$ev) return;
        $op = $ev['op'] ?? '';
        if ($op === 'command' && !empty($ev['srcId'])) {
            $this->clickedIds[$ev['srcId']] = true;
        } elseif ($op === 'changed' && !empty($ev['id'])) {
            $this->values[$ev['id']] = $ev['value'] ?? '';
        }
    }
}

function log(...$args): void {
    fwrite(STDERR, "[VTUI_LOG] " . implode(' ', $args) . "\n");
}

function run(callable $uiFunc, array $options = []): void {
    $session = new Session($options);
    $session->start();
    $u = new Ui($session);

    $step = function() use ($uiFunc, $u) {
        $uiFunc($u);
        $u->_sync();
    };

    try {
        $step();

        while ($ev = $session->recv()) {
            if (($ev['op'] ?? '') === 'closed' && ($ev['frameId'] ?? '') === $u->rootId) {
                break;
            }
            $u->_processEvent($ev);
            $step();
        }
    } finally {
        $session->close();
    }
}
