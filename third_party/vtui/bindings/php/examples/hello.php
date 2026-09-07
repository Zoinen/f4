<?php

require_once __DIR__ . '/../src/Vtui.php';

use function Vtui\run;
use Vtui\Ui;

run(function(Ui $u) {
    $u->dialog(" Hello vtui ", 40, function() use ($u) {
        $name = $u->edit("&Name:", "Type here...");
        if ($u->button("&Ok")) {
            $u->message(" Result ", "You typed:\n" . $name);
        }
    });
});
