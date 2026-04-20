import { useEffect, useState } from "react";
import {
  useCreateRole,
  useDeleteRole,
  useRole,
  useRoles,
  useUpdateRole,
} from "../api/queries";
import type { RoleSpec } from "../api/types";
import { RoleEditor } from "../components/roles/RoleEditor";
import { RoleDiagnostics } from "../components/roles/RoleDiagnostics";
import { RoleList } from "../components/roles/RoleList";

export function RolesPage() {
  const { data, isLoading } = useRoles();
  const createRole = useCreateRole();
  const updateRole = useUpdateRole();
  const deleteRole = useDeleteRole();
  const [selectedRoleID, setSelectedRoleID] = useState<string | null>(null);
  const [isCreatingNew, setIsCreatingNew] = useState(false);
  const [resetToken, setResetToken] = useState(0);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const roles = data?.roles ?? [];
  const diagnostics = data?.diagnostics ?? [];
  const selectedRoleSummary = roles.find((role) => role.role_id === selectedRoleID) ?? null;
  const selectedRoleDetails = useRole(selectedRoleID);
  const selectedRole = selectedRoleSummary
    ? {
        ...selectedRoleSummary,
        prompt: selectedRoleDetails.data?.prompt ?? "",
      }
    : null;

  useEffect(() => {
    if (!roles.length) {
      return;
    }
    if (!selectedRoleID && !isCreatingNew) {
      setSelectedRoleID(roles[0].role_id);
      return;
    }
    if (selectedRoleID && !roles.some((role) => role.role_id === selectedRoleID)) {
      setSelectedRoleID(roles[0].role_id);
      setIsCreatingNew(false);
    }
  }, [roles, selectedRoleID, isCreatingNew]);

  async function handleSave(role: RoleSpec) {
    try {
      if (selectedRoleID && !isCreatingNew) {
        setErrorMessage(null);
        const updated = await updateRole.mutateAsync({
          roleID: selectedRoleID,
          role,
        });
        setSelectedRoleID(updated.role_id);
        setIsCreatingNew(false);
        return;
      }
      setErrorMessage(null);
      const created = await createRole.mutateAsync(role);
      setSelectedRoleID(created.role_id);
      setIsCreatingNew(false);
    } catch (err) {
      setErrorMessage("Failed to save role.");
      console.error("Failed to save role:", err);
    }
  }

  async function handleDelete() {
    if (!selectedRoleID || isCreatingNew) {
      return;
    }
    try {
      setErrorMessage(null);
      await deleteRole.mutateAsync(selectedRoleID);
      setSelectedRoleID(null);
      setIsCreatingNew(false);
    } catch (err) {
      setErrorMessage("Failed to delete role.");
      console.error("Failed to delete role:", err);
    }
  }

  function handleNewRole() {
    setErrorMessage(null);
    setSelectedRoleID(null);
    setIsCreatingNew(true);
    setResetToken((value) => value + 1);
  }

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden lg:flex-row">
      <RoleList
        roles={roles}
        selectedRoleID={selectedRoleID}
        isLoading={isLoading}
        onSelectRole={(roleID) => {
          setErrorMessage(null);
          setSelectedRoleID(roleID);
          setIsCreatingNew(false);
        }}
        onNewRole={handleNewRole}
      />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <RoleDiagnostics diagnostics={diagnostics} />
        {errorMessage ? (
          <section
            className="shrink-0 border-b px-4 py-3 text-sm md:px-6"
            role="alert"
            style={{
              backgroundColor: "rgba(220, 38, 38, 0.04)",
              borderColor: "rgba(220, 38, 38, 0.18)",
              color: "var(--color-error)",
            }}
          >
            {errorMessage}
          </section>
        ) : null}
        <RoleEditor
          role={selectedRole}
          resetToken={resetToken}
          isSaving={createRole.isPending || updateRole.isPending}
          isDeleting={deleteRole.isPending}
          onSave={handleSave}
          onDelete={handleDelete}
        />
      </div>
    </div>
  );
}
