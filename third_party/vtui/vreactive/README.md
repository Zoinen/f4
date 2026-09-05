# vreactive - Reactive Layer for vgui & vtui

`vreactive` provides a lightweight, thread-safe, and cycle-protected reactive primitives library for Go UI frameworks (`vtui` and `vgui`).

## Features

- **`Property[T]`**: Reactive state container with subscription handlers.
- **`Computed[T]` / `Computed2[T]` / `ComputedIf[T]`**: Automatically recomputed properties derived from reactive dependencies, including declarative ternary expressions (`ComputedIf`).
- **`Bind[T]`**: One-way property synchronization.
- **`StateMachine`**: Declarative state transitions and property setters.
- **`Behavior[T]` & `Animator[T]`**: Smooth and discrete property transition animations (`SmoothBehavior`, `DiscreteBehavior`) supporting easing curves (`EaseOutBack`, `EaseInOutQuad`, `EaseOutBounce`, etc.).
- **`RGBInterpolator`**: Seamless 24-bit TrueColor animation and channel blending.
- **`Effect(fn, deps...)`**: Signal-based side-effects running initially and tracking multi-property changes.
- **`TwoWayBind`**: Cycle-safe bidirectional binding between state properties and external UI components.
- **`BindEnabled` / `BindVisible`**: One-line declarative adapters linking widget flags directly to boolean properties.
- **Cycle Detection**: Prevents infinite notification loops by enforcing a maximum call depth limit.
- **Thread Safety**: Mutex-protected reads/writes and `SafeSet` for asynchronous background goroutine updates via UI event queues.

## Usage Example

```go
nameProp := vreactive.NewProperty("Alice")
greetingProp := vreactive.Computed(nameProp, func(name string) string {
    return "Hello, " + name + "!"
})

// Reacting to changes
greetingProp.OnChange(func(val string) {
    fmt.Println(val)
})

nameProp.Set("Bob") // Prints "Hello, Bob!"
```
