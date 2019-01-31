package helm

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	rbacv1 "k8s.io/api/rbac/v1"
)

type RBACEntity interface {
	// The type of entity (user, group, or service-account)
	EntityType() string
	// A unique string to identify the entity
	EntityID() string
	// Represented as an RBAC subject
	Subject() rbacv1.Subject
}

// Represents an RBAC User
type UserInfo struct {
	Name string
}

func convertToUserInfos(users []string) []RBACEntity {
	out := []RBACEntity{}
	for _, user := range users {
		out = append(out, UserInfo{user})
	}
	return out
}

func (user UserInfo) EntityType() string {
	return "user"
}

func (user UserInfo) EntityID() string {
	return user.Name
}

func (user UserInfo) Subject() rbacv1.Subject {
	return rbacv1.Subject{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "User",
		Name:     user.Name,
	}
}

func (user UserInfo) String() string {
	return user.Name
}

// Represents an RBAC Group
type GroupInfo struct {
	Name string
}

func convertToGroupInfos(groups []string) []RBACEntity {
	out := []RBACEntity{}
	for _, group := range groups {
		out = append(out, GroupInfo{group})
	}
	return out
}

func (group GroupInfo) EntityType() string {
	return "group"
}

func (group GroupInfo) EntityID() string {
	return group.Name
}

func (group GroupInfo) Subject() rbacv1.Subject {
	return rbacv1.Subject{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Group",
		Name:     group.Name,
	}
}

func (group GroupInfo) String() string {
	return group.Name
}

// Represents a ServiceAccount
type ServiceAccountInfo struct {
	Name      string
	Namespace string
}

func convertToServiceAccountInfos(serviceAccounts []string) ([]RBACEntity, error) {
	out := []RBACEntity{}
	for _, serviceAccount := range serviceAccounts {
		serviceAccountInfo, err := ExtractServiceAccountInfo(serviceAccount)
		if err != nil {
			return out, err
		}
		out = append(out, serviceAccountInfo)
	}
	return out, nil
}

// ExtractServiceAccountInfo takes a service account identifier and extract out the namespace and name.
func ExtractServiceAccountInfo(serviceAccountID string) (ServiceAccountInfo, error) {
	splitServiceAccount := strings.Split(serviceAccountID, "/")
	if len(splitServiceAccount) != 2 {
		return ServiceAccountInfo{}, errors.WithStackTrace(InvalidServiceAccountInfo{serviceAccountID})
	}
	serviceAccountInfo := ServiceAccountInfo{
		Namespace: splitServiceAccount[0],
		Name:      splitServiceAccount[1],
	}
	return serviceAccountInfo, nil
}

func (serviceAccount ServiceAccountInfo) EntityType() string {
	return "service-account"
}

func (serviceAccount ServiceAccountInfo) EntityID() string {
	// We need this ID to include both the namespace and name, and be compatible with resource names.
	// Resource names can only have - or .
	return fmt.Sprintf("%s.%s", serviceAccount.Namespace, serviceAccount.Name)
}

func (serviceAccount ServiceAccountInfo) Subject() rbacv1.Subject {
	return rbacv1.Subject{
		APIGroup:  "",
		Kind:      "ServiceAccount",
		Name:      serviceAccount.Name,
		Namespace: serviceAccount.Namespace,
	}
}

func (serviceAccount ServiceAccountInfo) String() string {
	return fmt.Sprintf("%s/%s", serviceAccount.Namespace, serviceAccount.Name)
}
