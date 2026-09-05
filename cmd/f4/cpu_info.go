package main

// CPUInfo is what the InfoPanel's CPU section renders. Fields are
// filled only where the OS supplies them; the panel skips missing
// rows. Load and LoadAvg are kept as separate signals — Linux gives
// loadavg for free from /proc/loadavg, Windows and macOS produce an
// instantaneous percent.
type CPUInfo struct {
	Model         string
	PhysicalCores int        // real cores (sockets × cores-per-socket); 0 if unknown
	LogicalCores  int        // hardware threads visible to the OS (runtime.NumCPU baseline)
	FreqMHz       int        // nominal/max clock in MHz; 0 if unknown or fixed (Apple Silicon)
	CacheBytes    [4]uint64  // L1..L4 in bytes; 0-entries render as blank
	LoadAvg       [3]float64 // 1 / 5 / 15 minute averages (Linux, macOS)
	HasLoad       bool       // true when LoadAvg is populated
	Load          int        // 0..100 (Windows)
	HasLoadPct    bool       // true when Load is populated
}
