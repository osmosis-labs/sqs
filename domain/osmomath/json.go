package osmomath

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"

	sdkmath "cosmossdk.io/math"
	cosmosmath "github.com/osmosis-labs/osmosis/osmomath"
)

// JSONUnmarshalOptions decodes the exact Osmosis numeric types (cosmossdk.io/math.LegacyDec and Int,
// alias osmomath.Dec and osmomath.Int, plus osmomath.BigDec) straight from the token stream. Pass
// it to json.Unmarshal where large Osmosis models are decoded: the ingested SQS and tick models and
// the pool state files. Request decoding keeps the plain v2 defaults.
//
// Each type's UnmarshalJSON receives the already-scanned value and runs a nested json.Unmarshal
// only to extract the string, so every number is scanned twice and pays for a second decoder. Tick
// and orderbook models carry thousands of numbers, which made encoding/json/v2 slower than the
// previous jsoniter wrapper. Reading the string token once and parsing it with the owner's exported
// constructor removes that overhead. Accepted forms, precision and range remain those of
// sdkmath.LegacyNewDecFromStr, sdkmath.NewIntFromString and osmomath.NewBigDecFromStr, matching
// the dependencies' methods. Int keeps its base-0 grammar, but receives a fresh value
// instead of overwriting a big.Int that copies of the target may share.
//
// The option reaches only values decoded by the decoder it is passed to. Types whose own
// UnmarshalJSON calls json.Unmarshal internally (for example ingesttypes.TakerFeeMap) still decode
// their contents through the dependencies' methods: correct, just not accelerated.
var JSONUnmarshalOptions = json.WithUnmarshalers(json.JoinUnmarshalers(
	stringNumber(sdkmath.LegacyNewDecFromStr),
	stringNumber(parseInt),
	stringNumber(cosmosmath.NewBigDecFromStr),
))

// JSONMarshalOptions encodes sdkmath.LegacyDec and Int (alias osmomath.Dec and osmomath.Int)
// straight into the encoder. Pass it to json.Marshal where responses carry many such numbers; the
// Echo response serializer uses it.
//
// Each type's MarshalJSON formats the value (String for decimals, big.Int.MarshalText for integers),
// then runs a nested json.Marshal to quote it, and the outer encoder scans and copies those bytes
// again. Writing String() as a string token skips the nested encoder and intermediate slice. These
// formatters produce identical numeric text, so the JSON bytes are unchanged.
//
// BigDec is not registered: quote responses carry none, and the remaining endpoints that do (token
// prices) have not been measured to need it. Reuse this value rather than rebuilding it per call:
// json caches its per-type function lookups on it.
var JSONMarshalOptions = json.WithMarshalers(JSONMarshalers)

// JSONMarshalers are the marshalers behind JSONMarshalOptions. Callers that register marshalers
// of their own must join these into the same json.WithMarshalers option: a later WithMarshalers
// replaces an earlier one instead of adding to it.
var JSONMarshalers = json.JoinMarshalers(
	stringNumberTo[sdkmath.LegacyDec](),
	stringNumberTo[sdkmath.Int](),
)

// stringNumberTo encodes a non-nil value as a JSON string holding v.String(). A nil-backed value
// (the zero struct) falls through to the type's own MarshalJSON, which owns its representation of
// it (currently "0"; String would print "<nil>").
func stringNumberTo[T interface {
	IsNil() bool
	String() string
}]() *json.Marshalers {
	return json.MarshalToFunc(func(enc *jsontext.Encoder, v T) error {
		if v.IsNil() {
			return errors.ErrUnsupported // nothing written, so json falls back to the method
		}
		return enc.WriteToken(jsontext.String(v.String()))
	})
}

// parseInt adapts sdkmath.NewIntFromString, which reports failure as a bool, to an error.
func parseInt(s string) (sdkmath.Int, error) {
	v, ok := sdkmath.NewIntFromString(s)
	if !ok {
		return sdkmath.Int{}, fmt.Errorf("invalid integer or out of %d-bit range: %q", sdkmath.MaxBitLen, s)
	}
	return v, nil
}

// stringNumber decodes a JSON string with the owning package's exported parser. null and every
// other JSON kind fall through to the type's own UnmarshalJSON, so their behavior is not restated
// here. A failed string parse leaves dst unchanged. Fallback methods and the surrounding decode
// may still mutate their targets before an error; callers must discard failed decodes.
func stringNumber[T any](parse func(string) (T, error)) *json.Unmarshalers {
	return json.UnmarshalFromFunc(func(dec *jsontext.Decoder, dst *T) error {
		if dec.PeekKind() != jsontext.KindString {
			return errors.ErrUnsupported // nothing consumed, so json falls back to the method
		}
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}
		v, err := parse(tok.String())
		if err != nil {
			return err
		}
		*dst = v
		return nil
	})
}
