package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTLSSubjectInfoJsonOrgOrgUnit(t *testing.T) {
	t.Parallel()
	subjectInfo, err := parseOrCreateTLSSubjectInfo(`{"org": "Gruntwork", "org_unit": "Eng"}`)
	assert.NoError(t, err)
	assert.Equal(t, subjectInfo.Org, "Gruntwork")
	assert.Equal(t, subjectInfo.OrgUnit, "Eng")
}

func TestParseTLSSubjectInfoJsonOrganizationOrganizationalUnit(t *testing.T) {
	t.Parallel()
	subjectInfo, err := parseOrCreateTLSSubjectInfo(`{"organization": "Gruntwork", "organizational_unit": "Eng"}`)
	assert.NoError(t, err)
	assert.Equal(t, subjectInfo.Org, "Gruntwork")
	assert.Equal(t, subjectInfo.OrgUnit, "Eng")
}
