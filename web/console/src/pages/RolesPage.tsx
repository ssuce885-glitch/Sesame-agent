import { useEffect, useState } from "react";
import {
  useCreateRole,
  useDeleteRole,
  useRoles,
  useUpdateRole,
} from "../api/queries";
import type { RoleSpec } from "../api/types";
import { RoleEditor } from "../components/roles/RoleEditor";
import { RoleList } from "../components/roles/RoleList";

export function RolesPage() {
  const { data, isLoading } = useRoles();
  const createRole = useCreateRole();
  const updateRole = useUpdateRole();
  const deleteRole = useDeleteRole();
  const [selectedRoleID, setSelectedRoleID] = useState<string | null>(null);
  const [isCreatingNew, setIsCreatingNew] = useState(false);
  const [resetToken, setResetToken] = useState(0);

  const roles = data?.roles ?? [];
  const selectedRole = roles.find((role) => role.role_id === selectedRoleID) ?? null;

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
      if (selectedRole) {
        const updated = await updateRole.mutateAsync({
          roleID: selectedRole.role_id,
          role,
        });
        setSelectedRoleID(updated.role_id);
        setIsCreatingNew(false);
        return;
      }
      const created = await createRole.mutateAsync(role);
      setSelectedRoleID(created.role_id);
      setIsCreatingNew(false);
    } catch (err) {
      console.error("Failed to save role:", err);
    }
  }

  async function handleDelete() {
    if (!selectedRole) {
      return;
    }
    try {
      await deleteRole.mutateAsync(selectedRole.role_id);
      setSelectedRoleID(null);
      setIsCreatingNew(false);
    } catch (err) {
      console.error("Failed to delete role:", err);
    }
  }

  function handleNewRole() {
    setSelectedRoleID(null);
    setIsCreatingNew(true);
    setResetToken((value) => value + 1);
  }

  return (
    <div className="flex h-full overflow-hidden">
      <RoleList
        roles={roles}
        selectedRoleID={selectedRoleID}
        isLoading={isLoading}
        onSelectRole={(roleID) => {
          setSelectedRoleID(roleID);
          setIsCreatingNew(false);
        }}
        onNewRole={handleNewRole}
      />
      <RoleEditor
        role={selectedRole}
        resetToken={resetToken}
        isSaving={createRole.isPending || updateRole.isPending}
        isDeleting={deleteRole.isPending}
        onSave={handleSave}
        onDelete={handleDelete}
      />
    </div>
  );
}
