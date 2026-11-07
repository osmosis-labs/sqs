package osmomath_test

import (
	"encoding/json/v2"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"
	cosmosmath "github.com/osmosis-labs/osmosis/osmomath"

	osmojson "github.com/osmosis-labs/osmosis/v31/ingest/types/json"
	sqsosmomath "github.com/osmosis-labs/sqs/domain/osmomath"
)

// The oracle for every test here is the dependency's own UnmarshalJSON, reached by decoding the
// same document without JSONUnmarshalOptions. The adapter must accept exactly the same inputs and
// produce the same values. On failure callers discard the target, so its contents are not compared.

var (
	pow10_18 = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	// Largest LegacyDec: (2^256 * 10^18 - 1) * 10^-18.
	maxLegacyDec = sdkmath.LegacyNewDecFromBigIntWithPrec(
		new(big.Int).Sub(new(big.Int).Mul(new(big.Int).Lsh(big.NewInt(1), 256), pow10_18), big.NewInt(1)), 18).String()
	maxInt = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)).String()
	// One unit beyond the Int limit, and one 10^-18 unit beyond the LegacyDec limit.
	overflow256 = new(big.Int).Lsh(big.NewInt(1), 256).String()
	// Largest BigDec: a 1024-bit scaled magnitude with 36 fractional digits.
	maxBigDec = cosmosmath.NewBigDecFromBigIntWithPrec(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 1024), big.NewInt(1)), 36).String()
	// One 10^-36 unit beyond the BigDec limit, without exceeding its fractional precision.
	overflowBigDec = new(big.Rat).SetFrac(new(big.Int).Lsh(big.NewInt(1), 1024),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(36), nil)).FloatString(36)
)

// numberValues are JSON values (not documents) shared by every numeric type.
var numberValues = []string{
	// Valid in at least one type.
	`"0"`, `"-0"`, `"1"`, `"-1"`, `"007.5"`, `"123.456"`, `"-0.000000000000000001"`,
	`"0.123456789012345678"`, `"0.1234567890123456789"`,
	`"0.123456789012345678901234567890123456"`, `"0.1234567890123456789012345678901234567"`,
	`"` + maxLegacyDec + `"`, `"-` + maxLegacyDec + `"`, `"` + maxLegacyDec + `1"`,
	`"` + maxInt + `"`, `"-` + maxInt + `"`, `"` + maxInt + `0"`,
	`"` + maxBigDec + `"`, `"-` + maxBigDec + `"`, `"` + maxBigDec + `1"`,
	`"` + overflow256 + `"`, `"-` + overflow256 + `"`,
	`"` + overflowBigDec + `"`, `"-` + overflowBigDec + `"`,
	`"123456789012345678901234567890"`,
	// Dependency parser quirks that must be preserved, not fixed.
	`"+1.5"`, `"+.5"`, `"--1"`, `"+5"`, `"010"`, `"0x10"`, `"0b11"`, `"0o17"`, `"1_000"`, `"-_1"`,
	// Invalid grammar.
	`".5"`, `"5."`, `"."`, `"1.2.3"`, `"1e5"`, `" 1"`, `"1 "`, `""`, `"-"`, `"+"`, `"abc"`, `"NaN"`,
	// Escapes are unescaped before parsing.
	`"\u0031\u002e5"`, `"\u002d1"`, `"1\u002e5"`, `"\u0000"`,
	// Other JSON kinds fall through to the dependency's method.
	`null`, `1.5`, `15`, `true`, `false`, `{}`, `[]`, `["1"]`, `{"v":"1"}`,
}

// number is the part of the Osmosis numeric types' API the tests compare. String is canonical for
// each type (fixed precision for decimals), so equal strings mean equal values.
type number interface {
	IsNil() bool
	String() string
}

func requireSameNumber[T number](t *testing.T, want, got T) {
	t.Helper()
	require.Equal(t, want.IsNil(), got.IsNil())
	if !want.IsNil() {
		require.Equal(t, want.String(), got.String())
	}
}

// decodeBoth decodes doc with the dependency's methods (want) and with the adapter (got), and
// requires both to agree on success.
func decodeBoth[T any](t *testing.T, doc string, newTarget func() T) (want, got T, wantErr, gotErr error) {
	t.Helper()
	want, got = newTarget(), newTarget()
	wantErr = json.Unmarshal([]byte(doc), &want)
	gotErr = json.Unmarshal([]byte(doc), &got, sqsosmomath.JSONUnmarshalOptions)
	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("%s: method error %v, adapter error %v", doc, wantErr, gotErr)
	}
	return want, got, wantErr, gotErr
}

// requireSameCause compares the parser error and its location. The json.SemanticError wrapper
// describes the Go type differently for a method (T) and a type-specific function (*T).
func requireSameCause(t *testing.T, want, got error) {
	t.Helper()
	var wantSemantic, gotSemantic *json.SemanticError
	if !errors.As(want, &wantSemantic) {
		require.Equal(t, want.Error(), got.Error())
		return
	}
	require.ErrorAs(t, got, &gotSemantic)
	require.Equal(t, wantSemantic.JSONPointer, gotSemantic.JSONPointer)
	require.Equal(t, wantSemantic.Err, gotSemantic.Err)
}

// requireMatchesMethod runs every numberValues case for T in value, prepopulated value, pointer,
// slice and map positions. sameErrors reports whether the adapter's parse errors are the
// dependency's own (true for the decimals, whose methods call the same constructor).
func requireMatchesMethod[T number](t *testing.T, prepopulated func() T, sameErrors bool) {
	type valueField struct {
		V T `json:"v"`
	}
	type pointerField struct {
		P *T `json:"p"`
	}
	for _, value := range numberValues {
		t.Run(value, func(t *testing.T) {
			for _, initial := range []func() valueField{
				func() valueField { return valueField{} },
				func() valueField { return valueField{V: prepopulated()} },
			} {
				want, got, wantErr, gotErr := decodeBoth(t, `{"v":`+value+`}`, initial)
				if wantErr != nil {
					// Non-strings always reach the dependency's method.
					if sameErrors || !strings.HasPrefix(value, `"`) {
						requireSameCause(t, wantErr, gotErr)
					}
					continue
				}
				requireSameNumber(t, want.V, got.V)
			}

			want, got, wantErr, _ := decodeBoth(t, `{"p":`+value+`}`, func() pointerField { return pointerField{} })
			if wantErr == nil {
				require.Equal(t, want.P == nil, got.P == nil)
				if want.P != nil {
					requireSameNumber(t, *want.P, *got.P)
				}
			}

			wantSlice, gotSlice, wantErr, _ := decodeBoth(t, `[`+value+`,"2"]`, func() []T { return nil })
			if wantErr == nil {
				require.Len(t, gotSlice, len(wantSlice))
				for i := range wantSlice {
					requireSameNumber(t, wantSlice[i], gotSlice[i])
				}
			}

			wantMap, gotMap, wantErr, _ := decodeBoth(t, `{"a":`+value+`}`, func() map[string]T { return nil })
			if wantErr == nil {
				require.Len(t, gotMap, len(wantMap))
				for k := range wantMap {
					requireSameNumber(t, wantMap[k], gotMap[k])
				}
			}
		})
	}
}

func TestJSONUnmarshalOptions_LegacyDecMatchesMethod(t *testing.T) {
	requireMatchesMethod(t, func() sdkmath.LegacyDec { return sdkmath.LegacyMustNewDecFromStr("42.5") }, true)
}

func TestJSONUnmarshalOptions_BigDecMatchesMethod(t *testing.T) {
	requireMatchesMethod(t, func() cosmosmath.BigDec { return cosmosmath.MustNewBigDecFromStr("42.5") }, true)
}

func TestJSONUnmarshalOptions_IntMatchesMethod(t *testing.T) {
	// NewIntFromString reports failure as a bool, so the adapter writes its own message.
	requireMatchesMethod(t, func() sdkmath.Int { return sdkmath.NewInt(42) }, false)
}

// TestJSONUnmarshalOptions_PinnedValues pins dependency quirks the adapter deliberately keeps, so a
// dependency upgrade that changes them is noticed rather than silently absorbed.
func TestJSONUnmarshalOptions_PinnedValues(t *testing.T) {
	var decs struct {
		A, B, C, D sdkmath.LegacyDec
		E          cosmosmath.BigDec
	}
	doc := `{"A":"+1.5","B":"+.5","C":"--1","D":"0.000000000000000001","E":"--0.000000000000000000000000000000000001"}`
	require.NoError(t, json.Unmarshal([]byte(doc), &decs, sqsosmomath.JSONUnmarshalOptions))
	require.Equal(t, "1.500000000000000000", decs.A.String())
	require.Equal(t, "0.500000000000000000", decs.B.String())
	require.Equal(t, "1.000000000000000000", decs.C.String())
	require.Equal(t, "0.000000000000000001", decs.D.String())
	require.Equal(t, "0.000000000000000000000000000000000001", decs.E.String())

	var ints struct {
		Octal, Hex, Underscore, Max sdkmath.Int
	}
	doc = `{"Octal":"010","Hex":"0x10","Underscore":"1_000","Max":"` + maxInt + `"}`
	require.NoError(t, json.Unmarshal([]byte(doc), &ints, sqsosmomath.JSONUnmarshalOptions))
	require.Equal(t, "8", ints.Octal.String())
	require.Equal(t, "16", ints.Hex.String())
	require.Equal(t, "1000", ints.Underscore.String())
	require.Equal(t, maxInt, ints.Max.String())
}

// TestJSONUnmarshalOptions_DoesNotMutateSharedInt pins a deliberate difference from
// Int.UnmarshalJSON, which overwrites the big.Int that copies of the target share.
func TestJSONUnmarshalOptions_DoesNotMutateSharedInt(t *testing.T) {
	shared := sdkmath.NewInt(7)
	target := struct{ V sdkmath.Int }{V: shared}
	require.NoError(t, json.Unmarshal([]byte(`{"V":"5"}`), &target, sqsosmomath.JSONUnmarshalOptions))
	require.Equal(t, "5", target.V.String())
	require.Equal(t, "7", shared.String())
}

// TestJSONUnmarshalOptions_KeepsV2Strictness checks that the option changes only how numeric
// strings are parsed, not the document-level checks of encoding/json/v2.
func TestJSONUnmarshalOptions_KeepsV2Strictness(t *testing.T) {
	type model struct {
		D sdkmath.LegacyDec `json:"d"`
		I sdkmath.Int       `json:"i"`
		B cosmosmath.BigDec `json:"b"`
	}
	for name, doc := range map[string]string{
		"duplicate name":          `{"d":"1","d":"2"}`,
		"invalid UTF-8 in dec":    "{\"d\":\"1\xff\"}",
		"invalid UTF-8 in int":    "{\"i\":\"1\xff\"}",
		"invalid UTF-8 in bigdec": "{\"b\":\"1\xff\"}",
		"trailing data":           `{"d":"1"} {}`,
		"truncated":               `{"d":"1`,
	} {
		t.Run(name, func(t *testing.T) {
			var m model
			require.Error(t, json.Unmarshal([]byte(doc), &m, sqsosmomath.JSONUnmarshalOptions))
		})
	}
}

// TestJSONUnmarshalOptions_MatchesOsmosisWrapper compares with the previous codec on the forms the
// Osmosis node writes.
func TestJSONUnmarshalOptions_MatchesOsmosisWrapper(t *testing.T) {
	type model struct {
		D []sdkmath.LegacyDec `json:"d"`
		I []sdkmath.Int       `json:"i"`
		B []cosmosmath.BigDec `json:"b"`
	}
	doc := `{"d":["0","1.5","-0.000000000000000001","` + maxLegacyDec + `"],` +
		`"i":["0","-7","` + maxInt + `"],` +
		`"b":["0","168.384233866094296071718173469546211456","` + maxBigDec + `"]}`
	var want, got model
	require.NoError(t, osmojson.Unmarshal([]byte(doc), &want))
	require.NoError(t, json.Unmarshal([]byte(doc), &got, sqsosmomath.JSONUnmarshalOptions))
	wantBz, err := osmojson.Marshal(want)
	require.NoError(t, err)
	gotBz, err := osmojson.Marshal(got)
	require.NoError(t, err)
	require.Equal(t, string(wantBz), string(gotBz))
}

// FuzzJSONUnmarshalOptions decodes arbitrary JSON values with and without the option and requires
// the same outcome, catching drift between a dependency's UnmarshalJSON and its constructor.
func FuzzJSONUnmarshalOptions(f *testing.F) {
	for _, value := range numberValues {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		fuzzMatchesMethod[sdkmath.LegacyDec](t, value)
		fuzzMatchesMethod[sdkmath.Int](t, value)
		fuzzMatchesMethod[cosmosmath.BigDec](t, value)
	})
}

func fuzzMatchesMethod[T number](t *testing.T, value string) {
	doc := []byte(`{"V":` + value + `}`)
	var want, got struct{ V T }
	wantErr := json.Unmarshal(doc, &want)
	gotErr := json.Unmarshal(doc, &got, sqsosmomath.JSONUnmarshalOptions)
	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("%q into %T: method error %v, adapter error %v", value, want.V, wantErr, gotErr)
	}
	if wantErr == nil {
		requireSameNumber(t, want.V, got.V)
	}
}

// TestJSONUnmarshalOptions_ConcurrentUse shares the package-level option, as concurrent ingest
// workers do. Run with -race.
func TestJSONUnmarshalOptions_ConcurrentUse(t *testing.T) {
	type model struct {
		D sdkmath.LegacyDec `json:"d"`
		I sdkmath.Int       `json:"i"`
		B cosmosmath.BigDec `json:"b"`
	}
	doc := []byte(`{"d":"1.5","i":"7","b":"0.25"}`)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			for range 100 {
				var m model
				if err := json.Unmarshal(doc, &m, sqsosmomath.JSONUnmarshalOptions); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
