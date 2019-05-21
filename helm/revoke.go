package helm

import (
	"github.com/gruntwork-io/gruntwork-cli/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		"Revoking access Tiller server deployed in namespace %s to:",
		tillerNamespace,
	)
	logEntities(rbacGroups, rbacUsers, serviceAccounts)

	logger.Info("Checking if Tiller is deployed in the namespace.")
	if err := validateTillerDeployed(kubectlOptions, tillerNamespace); err != nil {
		logger.Errorf("Did not find a deployed Tiller instance in the namespace %s.", tillerNamespace)
		return err
	}
	logger.Infof("Found a valid Tiller instance in the namespace %s.", tillerNamespace)

	if len(rbacGroups) > 0 {
		logger.Infof("Revoking access from RBAC groups")
		if err := revokeAccessFromRBACEntities(kubectlOptions, tillerNamespace, convertToGroupInfos(rbacGroups)); err != nil {
			return err
		}
		logger.Infof("Successfully revoked access from RBAC groups")
	} else {
		logger.Infof("No RBAC groups to revoke - skipping")
	}

	if len(rbacUsers) > 0 {
		logger.Infof("Revoking access from RBAC users")
		if err := revokeAccessFromRBACEntities(kubectlOptions, tillerNamespace, convertToGroupInfos(rbacUsers)); err != nil {
			return err
		}
		logger.Infof("Successfully revoked access from RBAC users")
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
			return err
		}
		logger.Infof("Successfully revoked access from Service Accounts")
	} else {
		logger.Infof("No Service Accounts to revoke - skipping")
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

	numEntities := len(rbacEntities)
	for idx, rbacEntity := range rbacEntities {
		logger.Infof("Revoking access to entity %s (%d of %d entities)", rbacEntity, idx+1, numEntities)

		logger.Infof("Deleting entity role")
		if err := deleteEntityRoleAndBinding(kubectlOptions, tillerNamespace, rbacEntity); err != nil {
			logger.Errorf("Unable to delete role for entity %s from namespace %s", rbacEntity.Subject().Name, tillerNamespace)
			return err
		}

		logger.Infof("Deleting entity keypair from secrets")
		if err := deleteEntityTLS(kubectlOptions, tillerNamespace, rbacEntity); err != nil {
			logger.Errorf("Unable to delete TLS keypair for entity %s from namespace %s", rbacEntity.Subject().Name, tillerNamespace)
			return err
		}
	}
	return nil
}

// deleteEntityRoleandBinding deletes the RBAC role and role binding associated with a provided entity
func deleteEntityRoleAndBinding(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	rbacEntity RBACEntity,
) error {
	logger := logging.GetProjectLogger()

	client, err := kubectl.GetKubernetesClientFromOptions(kubectlOptions)
	if err != nil {
		return err
	}

	emptyOptions := &metav1.DeleteOptions{}
	roleName := getTillerAccessRoleName(rbacEntity.EntityID(), tillerNamespace)
	err = client.RbacV1().Roles(tillerNamespace).Delete(roleName, emptyOptions)
	if err != nil {
		logger.Errorf("Error deleting RBAC role: %s", err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully deleted RBAC role %s", roleName)

	roleBindingName := getTillerAccessRoleBindingName(rbacEntity.EntityID(), roleName)
	err = client.RbacV1().RoleBindings(tillerNamespace).Delete(roleBindingName, emptyOptions)
	if err != nil {
		logger.Errorf("Error deleting RBAC role binding: %s", err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully deleted RBAC role binding %s", roleName)
	return nil
}

// deleteEntityTLS deletes the TLS keypair associated with a provided entity
func deleteEntityTLS(
	kubectlOptions *kubectl.KubectlOptions,
	tillerNamespace string,
	rbacEntity RBACEntity,
) error {
	logger := logging.GetProjectLogger()

	secretName := getTillerClientCertSecretName(rbacEntity.EntityID())
	err := kubectl.DeleteSecret(kubectlOptions, tillerNamespace, secretName)
	if err != nil {
		logger.Errorf("Error deleting client cert: %s", err)
		return errors.WithStackTrace(err)
	}
	logger.Infof("Successfully deleted client cert from secret %s", secretName)
	return nil
}
