package helm

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

type RevocationErrors struct {
	errors []error
}

func (err RevocationErrors) Error() string {
	messages := []string{
		fmt.Sprintf("%d errors found while revoking RBAC entities:", len(err.errors)),
	}

	for _, individualErr := range err.errors {
		messages = append(messages, individualErr.Error())
	}
	return strings.Join(messages, "\n")
}

func (err RevocationErrors) AddError(newErr error) {
	err.errors = append(err.errors, newErr)
}

func (err RevocationErrors) IsEmpty() bool {
	return len(err.errors) == 0
}

func NewRevocationErrors() RevocationErrors {
	return RevocationErrors{[]error{}}
}

// RevokeAccess revokes access to a Tiller pod from a provided RBAC user, group, or serviceaccount in a
// provided Tiller namespace by deleting the secret, role, and rolebindings associated with said entities.
// Note that due to limitations in the Go TLS library used by helm, helm/tiller does not support checking certificate revocation lists. As a
// consequence, the signed TLS certificate will continue to be trusted by Tiller after running "kubergrunt helm revoke" since
// it was signed by the Tiller CA. However, the user's authorizations are removed by way of deleting the role and role binding and access is
// effectively removed.
// See https://github.com/helm/helm/issues/4273
func RevokeAccess(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	rbacGroups []string,
	rbacUsers []string,
	serviceAccounts []string,
) error {
	logger := logging.GetProjectLogger()
	logger.Infof(
		"Revoking access to Tiller server deployed in namespace %s to:",
		tillerNamespace,
	)
	logEntities(rbacGroups, rbacUsers, serviceAccounts)

	logger.Info("Checking if Tiller is deployed in the namespace.")
	if err := validateTillerDeployed(kubectlOptions, tillerNamespace); err != nil {
		logger.Errorf("Did not find a deployed Tiller instance in the namespace %s.", tillerNamespace)
		return err
	}
	logger.Infof("Found a valid Tiller instance in the namespace %s.", tillerNamespace)

	errList := NewRevocationErrors()
	if len(rbacGroups) > 0 {
		logger.Infof("Revoking access from RBAC groups")
		if err := revokeAccessFromRBACEntities(kubectlOptions, tillerNamespace, convertToGroupInfos(rbacGroups)); err != nil {
			errList.AddError(err)
		} else {
			logger.Infof("Successfully revoked access from RBAC groups")
		}
	} else {
		logger.Infof("No RBAC groups to revoke - skipping")
	}

	if len(rbacUsers) > 0 {
		logger.Infof("Revoking access from RBAC users")
		if err := revokeAccessFromRBACEntities(kubectlOptions, tillerNamespace, convertToGroupInfos(rbacUsers)); err != nil {
			errList.AddError(err)
		} else {
			logger.Infof("Successfully revoked access from RBAC users")
		}
	} else {
		logger.Infof("No RBAC users to revoke - skipping")
	}

	if len(serviceAccounts) > 0 {
		logger.Infof("Revoking access from RBAC Service Accounts")
		serviceAccountInfos, err := convertToServiceAccountInfos(serviceAccounts)
		if err != nil {
			return err
		}
		if err := revokeAccessFromRBACEntities(kubectlOptions, tillerNamespace, serviceAccountInfos); err != nil {
			logger.Errorf("Error revoking access from Service Accounts: %s", err)
			errList.AddError(err)
		} else {
			logger.Infof("Successfully revoked access from Service Accounts")
		}
	} else {
		logger.Infof("No Service Accounts to revoke - skipping")
	}

	if !errList.IsEmpty() {
		logger.Error("Encountered revocation errors")
		return errors.WithStackTrace(errList)
	}
	return nil
}

// revokeAccessFromRBACEntities will revoke access to the deployed Tiller server to the provided RBAC groups.
// The revocation process is as follows:
// For each RBAC entity:
// - Discover and remove the role and rolebinding
// - Discover and remove the signed TLS keypair
func revokeAccessFromRBACEntities(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	rbacEntities []RBACEntity,
) error {
	logger := logging.GetProjectLogger()

	errList := NewRevocationErrors()
	numEntities := len(rbacEntities)
	for idx, entity := range rbacEntities {
		logger.Infof("Revoking access from entity %s (%d of %d entities)", entity, idx+1, numEntities)
		err := revokeAccessFromRBACEntity(kubectlOptions, tillerNamespace, entity)
		if err != nil {
			errList.AddError(err)
		}
	}
	if !errList.IsEmpty() {
		logger.Error("Encountered revocation errors")
		return errors.WithStackTrace(errList)
	}
	return nil
}

func revokeAccessFromRBACEntity(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	entity RBACEntity,
) error {
	logger := logging.GetProjectLogger()

	roleName := getTillerAccessRoleName(entity.EntityID(), tillerNamespace)

	logger.Infof("Deleting entity role %s", roleName)
	err := deleteEntityRole(kubectlOptions, tillerNamespace, roleName, entity)
	if err == nil {
		logger.Infof("Deleted role for entity %s", entity.Subject().Name)
	} else if err, ok := err.(*ResourceDoesNotExistError); ok {
		logger.Warningf("Unable to delete role for entity %s from namespace %s", entity.Subject().Name, tillerNamespace)
	} else {
		return err
	}

	logger.Infof("Deleting entity role binding")
	err = deleteEntityRoleBinding(kubectlOptions, tillerNamespace, roleName, entity)
	if err == nil {
		logger.Infof("Deleted role binding for entity %s", entity.Subject().Name)
	} else if err, ok := err.(*ResourceDoesNotExistError); ok {
		logger.Warningf("Unable to delete role binding for entity %s from namespace %s", entity.Subject().Name, tillerNamespace)
	} else {
		return err
	}

	logger.Infof("Deleting entity keypair from secrets")
	err = deleteEntityTLS(kubectlOptions, tillerNamespace, entity)
	if err == nil {
		logger.Infof("Deleted role binding for entity %s", entity.Subject().Name)
	} else if err, ok := err.(*ResourceDoesNotExistError); ok {
		logger.Warningf("Unable to delete TLS keypair for entity %s from namespace %s", entity.Subject().Name, tillerNamespace)
	} else {
		return err
	}

	return nil
}

type ResourceDoesNotExistError struct {
	Resource string
	Name     string
}

func (d *ResourceDoesNotExistError) Error() string {
	return fmt.Sprintf("%s %s does not exist", d.Resource, d.Name)
}

// deleteEntityRole deletes the RBAC role associated with a provided entity
func deleteEntityRole(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	roleName string,
	rbacEntity RBACEntity,
) error {
	logger := logging.GetProjectLogger()

	labels := getTillerRoleLabels(rbacEntity.EntityID(), tillerNamespace)
	filters := kubectl.LabelsToListOptions(labels)
	roles, err := kubectl.ListRoles(kubectlOptions, tillerNamespace, filters)
	if err != nil {
		logger.Errorf("Error checking for role: %s", err)
		return err
	}
	if len(roles) == 0 {
		return &ResourceDoesNotExistError{"Role", roleName}
	}

	logger.Infof("Deleting role %s", roleName)
	err = kubectl.DeleteRole(kubectlOptions, tillerNamespace, roleName)
	if err != nil {
		logger.Errorf("Error deleting RBAC role: %s", err)
		return err
	}
	logger.Infof("Successfully deleted RBAC role %s", roleName)

	return nil
}

// deleteEntityRoleBinding deletes the RBAC role binding associated with a provided entity
func deleteEntityRoleBinding(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	roleName string,
	rbacEntity RBACEntity,
) error {
	logger := logging.GetProjectLogger()

	roleBindingName := getTillerAccessRoleBindingName(rbacEntity.EntityID(), roleName)

	labels := getTillerRoleBindingLabels(rbacEntity.EntityID(), tillerNamespace)
	filters := kubectl.LabelsToListOptions(labels)
	bindings, err := kubectl.ListRoleBindings(kubectlOptions, tillerNamespace, filters)
	if err != nil {
		logger.Errorf("Error checking for role binding: %s", err)
		return err
	}
	if len(bindings) == 0 {
		return &ResourceDoesNotExistError{"RoleBinding", roleName}
	}

	logger.Infof("Deleting role binding %s", roleBindingName)
	err = kubectl.DeleteRoleBinding(kubectlOptions, tillerNamespace, roleBindingName)
	if err != nil {
		logger.Errorf("Error deleting RBAC role: %s", err)
		return err
	}
	logger.Infof("Successfully deleted RBAC role %s", roleBindingName)

	return nil
}

// deleteEntityTLS deletes the Kubernetes Secret holding the client TLS keypair associated with a provided entity
func deleteEntityTLS(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	rbacEntity RBACEntity,
) error {
	logger := logging.GetProjectLogger()

	secretName := getTillerClientCertSecretName(rbacEntity.EntityID())

	labels := getTillerClientCertSecretLabels(rbacEntity.EntityID(), tillerNamespace)
	filters := kubectl.LabelsToListOptions(labels)
	secrets, err := kubectl.ListSecrets(kubectlOptions, tillerNamespace, filters)
	if len(secrets) == 0 {
		return &ResourceDoesNotExistError{"Secret", secretName}
	}
	if err != nil {
		logger.Errorf("Error checking for secret: %s", err)
		return err
	}
	err = kubectl.DeleteSecret(kubectlOptions, tillerNamespace, secretName)
	if err != nil {
		logger.Warningf("Error deleting client cert secret: %s", err)
	} else {
		logger.Infof("Successfully deleted client cert from secret %s", secretName)
	}
	return nil
}
