package protocol

const Forward = "wormhole-forward-v1"

type Request struct {
	Name  string `json:"name"`
	Local string `json:"local"`
}
type Registration struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	URL     string `json:"url"`
}
