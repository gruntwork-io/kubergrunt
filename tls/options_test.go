package tls

import (
	"testing"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/stretchr/testify/assert"
)

func TestTLSOptionsValidateAcceptsAllKnownAlgorithms(t *testing.T) {
	for _, algorithm := range PrivateKeyAlgorithms {
		options := TLSOptions{
			PrivateKeyAlgorithm: algorithm,
			ECDSACurve:          P224Curve,
			RSABits:             MinimumRSABits,
		}
		assert.NoError(t, options.Validate())
	}
}

func TestTLSOptionsValidateAcceptsAllKnownCurves(t *testing.T) {
	for _, curve := range KnownCurves {
		options := TLSOptions{
			PrivateKeyAlgorithm: ECDSAAlgorithm,
			ECDSACurve:          curve,
			RSABits:             MinimumRSABits,
		}
		assert.NoError(t, options.Validate())
	}
}

func TestTLSOptionsValidateAcceptsUnknownCurveWhenAlgorithmIsRSA(t *testing.T) {
	options := TLSOptions{
		PrivateKeyAlgorithm: RSAAlgorithm,
		ECDSACurve:          "P0",
		RSABits:             MinimumRSABits,
	}
	assert.NoError(t, options.Validate())
}

func TestTLSOptionsValidateAcceptsRSABitsAboveMinimum(t *testing.T) {
	options := TLSOptions{
		PrivateKeyAlgorithm: RSAAlgorithm,
		ECDSACurve:          P224Curve,
		RSABits:             4096,
	}
	assert.NoError(t, options.Validate())
}

func TestTLSOptionsValidateAcceptsRSABitsBelowMinimumWhenAlgorithmIsECDSA(t *testing.T) {
	options := TLSOptions{
		PrivateKeyAlgorithm: ECDSAAlgorithm,
		ECDSACurve:          P224Curve,
		RSABits:             1,
	}
	assert.NoError(t, options.Validate())

}

func TestTLSOptionsValidateRejectsUnknownAlgorithms(t *testing.T) {
	options := TLSOptions{
		PrivateKeyAlgorithm: "UNKNOWN",
		ECDSACurve:          P224Curve,
		RSABits:             MinimumRSABits,
	}
	err := options.Validate()
	assert.Error(t, err)
	err = errors.Unwrap(err)
	switch err.(type) {
	case UnknownPrivateKeyAlgorithm:
	default:
		t.Fatalf("Wrong validation error type: %s", err)
	}
}

func TestTLSOptionsValidateRejectsUnknownCurves(t *testing.T) {
	options := TLSOptions{
		PrivateKeyAlgorithm: ECDSAAlgorithm,
		ECDSACurve:          "UNKNOWN",
		RSABits:             MinimumRSABits,
	}
	err := options.Validate()
	assert.Error(t, err)
	err = errors.Unwrap(err)
	switch err.(type) {
	case UnknownECDSACurveError:
	default:
		t.Fatalf("Wrong validation error type: %s", err)
	}
}

func TestTLSOptionsValidateRejectsRSABitsBelowMinimum(t *testing.T) {
	options := TLSOptions{
		PrivateKeyAlgorithm: RSAAlgorithm,
		ECDSACurve:          P224Curve,
		RSABits:             2047,
	}
	err := options.Validate()
	assert.Error(t, err)
	err = errors.Unwrap(err)
	switch err.(type) {
	case RSABitsTooLow:
	default:
		t.Fatalf("Wrong validation error type: %s", err)
	}
}
