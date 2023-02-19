package sequence

type SequenceSchema struct {
	Title    string   `toml:"title"`
	Sequence Sequence `toml:"sequence"`
}

type Item struct {
	Value          int    `toml:"value"`
	Timestamp      int64  `toml:"timestamp"`
	Imagesetconfig string `toml:"imagesetconfig"`
	Current        bool   `toml:"current"`
}

type Sequence struct {
	Owner string `toml:"owner"`
	Item  []Item `toml:"item"`
}
