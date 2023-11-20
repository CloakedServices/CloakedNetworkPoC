package cashu

import (
	"encoding/base64"
	"encoding/json"
	"log"
)

type Proof struct {
	Id     string `json:"id"`
	Amount uint64 `json:"amount"`
	Secret string `json:"secret"`
	C      string `json:"C"`
}
type Tokens struct {
	Token []Token `json:"token"`
	Memo  string  `json:"memo"`
}
type Proofs struct {
	ID     string `json:"id"`
	Amount int    `json:"amount"`
	Secret string `json:"secret"`
	C      string `json:"C"`
}
type Token struct {
	Mint   string  `json:"mint"`
	Proofs []Proof `json:"proofs"`
}

// Cashu helper functions
func NewTokens(t string) *Tokens {
	token := &Tokens{}
	decodedCoin, err := base64.URLEncoding.DecodeString(t[len("cashuA"):])
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(decodedCoin, &token)
	if err != nil {
		log.Fatal(err)
	}
	return token
}
