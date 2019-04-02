package tls

import (
	"crypto/x509/pkix"
	"time"
)

func SampleTlsOptions(algorithm string) TLSOptions {
	options := TLSOptions{
		DistinguishedName: pkix.Name{
			CommonName:         "gruntwork.io",
			Organization:       []string{"Gruntwork"},
			OrganizationalUnit: []string{"IT"},
			Locality:           []string{"Phoenix"},
			Province:           []string{"AZ"},
			Country:            []string{"US"},
		},
		ValidityTimeSpan:    1 * time.Hour,
		PrivateKeyAlgorithm: algorithm,
		RSABits:             2048,
		ECDSACurve:          P256Curve,
	}
	return options
}
