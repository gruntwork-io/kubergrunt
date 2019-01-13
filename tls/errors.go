package tls

import (
	"fmt"
)

// UnknownECDSACurveError is returned when an unknown ecdsa curve is requested.
type UnknownECDSACurveError struct {
	Curve string
}

func (err UnknownECDSACurveError) Error() string {
	return fmt.Sprintf("Unrecognized elliptic curve %s when generating ECDSA key pair.", err.Curve)
}

// RSABitsTooLow is returned when the requested RSA key length is too low.
type RSABitsTooLow struct {
	RSABits int
}

func (err RSABitsTooLow) Error() string {
	return fmt.Sprintf("RSA Key length of %d is too low. Choose at least 2048.", err.RSABits)
}
