<?php

require_once __DIR__ . '/../src/Vtui.php';

use Vtui\Ui;
use Vtui\Session;

class MockSession extends Session {
    public array $mounted = [];
    public array $messages = [];

    public function __construct() {}
    public function start(): void {}
    public function mount(string $frameId, array $tree): void {
        $this->mounted[] = ['frameId' => $frameId, 'tree' => $tree];
    }
    public function message(string $title, string $text, array $buttons = ['&Ok']): void {
        $this->messages[] = ['title' => $title, 'text' => $text, 'buttons' => $buttons];
    }
    public function close(): void {}
}

function assertStrict(bool $condition, string $msg) {
    if (!$condition) {
        fwrite(STDERR, "TEST FAILED: {$msg}\n");
        exit(1);
    }
}

// 1. Test immediate-mode tree construction
$session = new MockSession();
$u = new Ui($session);

$u->dialog(" Test Dialog ", 40, function() use ($u) {
    $name = $u->edit("&Name:", "Alice");
    assertStrict($name === "Alice", "Expected default text 'Alice', got '{$name}'");
    $clicked = $u->button("&Submit");
    assertStrict($clicked === false, "Button should not be clicked initially");
});

$u->_sync();

assertStrict(count($session->mounted) === 1, "Expected 1 mounted frame");
assertStrict($session->mounted[0]['frameId'] === 'mainDlg', "Expected frameId mainDlg");
assertStrict($session->mounted[0]['tree']['type'] === 'Dialog', "Expected Dialog root node");
assertStrict(count($session->mounted[0]['tree']['children']) === 2, "Expected 2 children in dialog");

// 2. Test event processing
$u->_processEvent(['op' => 'changed', 'id' => 'edit_Name:', 'value' => 'Bob']);
$u->_processEvent(['op' => 'command', 'srcId' => 'btn_Submit', 'cmd' => 1000]);

$submitted = false;
$u->dialog(" Test Dialog ", 40, function() use ($u, &$submitted) {
    $name = $u->edit("&Name:", "Alice");
    assertStrict($name === "Bob", "Expected updated text 'Bob', got '{$name}'");
    if ($u->button("&Submit")) {
        $submitted = true;
    }
});

assertStrict($submitted === true, "Expected button click to trigger after command event");

echo "PHP bindings unit test passed.\n";
exit(0);
