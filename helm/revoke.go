package helm

import (
	"fmt"

	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/kubergrunt/kubectl"
	"github.com/gruntwork-io/kubergrunt/logging"
)

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

	rbacEntities := []RBACEntity{}

	if len(rbacGroups) > 0 {
		for _, entity := range convertToGroupInfos(rbacGroups) {
			rbacEntities = append(rbacEntities, entity)
		}
	}

	if len(rbacUsers) > 0 {
		for _, entity := range convertToGroupInfos(rbacUsers) {
			rbacEntities = append(rbacEntities, entity)
		}
	}

	if len(serviceAccounts) > 0 {
		serviceAccountInfos, err := convertToServiceAccountInfos(serviceAccounts)
		if err != nil {
			return err
		}
		for _, entity := range serviceAccountInfos {
			rbacEntities = append(rbacEntities, entity)
		}
	}

	logger.Infof("Revoking access to Tiller from %d RBAC entities", len(rbacEntities))
	if err := revokeAccessFromRBACEntities(kubectlOptions, tillerNamespace, rbacEntities); err != nil {
		logger.Errorf("Encountered error while trying to revoke access to Tiller in Namespace %s from %d entities", tillerNamespace, len(rbacEntities))
		return err
	}
	logger.Infof("Successfully revoked access to Tiller")
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

	errList := MultiHelmError{Action: "revoke access"}
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
		logger.Warningf("Role for entity %s does not exist in namespace %s", entity.Subject().Name, tillerNamespace)
		logger.Warning("Ignoring error")
	} else {
		logger.Errorf("Unable to delete role for entity %s from namespace %s", entity.Subject().Name, tillerNamespace)
		return err
	}

	logger.Infof("Deleting entity role binding")
	err = deleteEntityRoleBinding(kubectlOptions, tillerNamespace, roleName, entity)
	if err == nil {
		logger.Infof("Deleted role binding for entity %s", entity.Subject().Name)
	} else if err, ok := err.(*ResourceDoesNotExistError); ok {
		logger.Warningf("RoleBinding for entity %s does not exist in namespace %s", entity.Subject().Name, tillerNamespace)
		logger.Warning("Ignoring error")
	} else {
		logger.Errorf("Unable to delete role binding for entity %s from namespace %s", entity.Subject().Name, tillerNamespace)
		return err
	}

	logger.Infof("Deleting Secrets containing entity keypair")
	err = deleteEntityTLS(kubectlOptions, tillerNamespace, entity)
	if err == nil {
		logger.Infof("Deleted TLS keypair secret for entity %s", entity.Subject().Name)
	} else if err, ok := err.(*ResourceDoesNotExistError); ok {
		logger.Warningf("TLS keypair Secret for entity %s does not exist in namespace %s", entity.Subject().Name, tillerNamespace)
		logger.Warning("Ignoring error")
	} else {
		logger.Errorf("Unable to delete TLS keypair Secret for entity %s from namespace %s", entity.Subject().Name, tillerNamespace)
		return err
	}

	return nil
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
		logger.Errorf("Error checking for existence of role: %s", err)
		return err
	}
	if len(roles) == 0 {
		return &ResourceDoesNotExistError{Resource: "Role", Name: roleName}
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
		logger.Errorf("Error checking for existence of role binding: %s", err)
		return err
	}
	if len(bindings) == 0 {
		return &ResourceDoesNotExistError{Resource: "RoleBinding", Name: roleName}
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
	if err != nil {
		logger.Errorf("Error checking for existence of secret: %s", err)
		return err
	}
	if len(secrets) == 0 {
		return &ResourceDoesNotExistError{Resource: "Secret", Name: secretName}
	}

	err = kubectl.DeleteSecret(kubectlOptions, tillerNamespace, secretName)
	if err != nil {
		logger.Errorf("Error deleting client cert secret: %s", err)
		return err
	}
	logger.Infof("Successfully deleted client cert from secret %s", secretName)
	return nil
}
