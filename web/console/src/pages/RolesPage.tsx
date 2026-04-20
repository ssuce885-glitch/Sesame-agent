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
  const [editorRole, setEditorRole] = useState<RoleSpec | null>(null);

  const roles = data?.roles ?? [];
  const diagnostics = data?.diagnostics ?? [];
  const selectedRoleDetails = useRole(selectedRoleID);
  const hasSelectedRoleLoadError =
    !isCreatingNew &&
    selectedRoleID !== null &&
    roles.length > 0 &&
    editorRole === null &&
    selectedRoleDetails.isError;
  const isLoadingSelectedRole =
    !isCreatingNew &&
    roles.length > 0 &&
    editorRole === null &&
    !hasSelectedRoleLoadError;

  useEffect(() => {
    if (!roles.length) {
      return;
    }
    if (!selectedRoleID && !isCreatingNew) {
      setEditorRole(null);
      setSelectedRoleID(roles[0].role_id);
      return;
    }
    if (selectedRoleID && !roles.some((role) => role.role_id === selectedRoleID)) {
      setEditorRole(null);
      setSelectedRoleID(roles[0].role_id);
      setIsCreatingNew(false);
    }
  }, [roles, selectedRoleID, isCreatingNew]);

  useEffect(() => {
    if (isCreatingNew || !selectedRoleID) {
      setEditorRole(null);
      return;
    }
    const detail = selectedRoleDetails.data;
    if (!detail || detail.role_id !== selectedRoleID) {
      return;
    }
    setEditorRole((current) => {
      if (current?.role_id === detail.role_id) {
        return current;
      }
      return detail;
    });
  }, [isCreatingNew, selectedRoleID, selectedRoleDetails.data]);

  async function handleSave(role: RoleSpec) {
    try {
      if (selectedRoleID && !isCreatingNew) {
        setErrorMessage(null);
        const updated = await updateRole.mutateAsync({
          roleID: selectedRoleID,
          role,
        });
        setEditorRole(updated);
        setSelectedRoleID(updated.role_id);
        setIsCreatingNew(false);
        return;
      }
      setErrorMessage(null);
      const created = await createRole.mutateAsync(role);
      setEditorRole(created);
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
      setEditorRole(null);
      setSelectedRoleID(null);
      setIsCreatingNew(false);
    } catch (err) {
      setErrorMessage("Failed to delete role.");
      console.error("Failed to delete role:", err);
    }
  }

  function handleNewRole() {
    setErrorMessage(null);
    setEditorRole(null);
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
          setEditorRole(null);
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
        {isLoadingSelectedRole ? (
          <section className="flex-1 overflow-y-auto p-4 md:p-6" style={{ backgroundColor: "var(--color-bg)" }}>
            <div
              className="max-w-3xl rounded-xl p-5 text-sm"
              style={{
                backgroundColor: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                color: "var(--color-text-muted)",
              }}
            >
              Loading role...
            </div>
          </section>
        ) : hasSelectedRoleLoadError ? (
          <section className="flex-1 overflow-y-auto p-4 md:p-6" style={{ backgroundColor: "var(--color-bg)" }}>
            <div
              className="max-w-3xl rounded-xl p-5 text-sm"
              style={{
                backgroundColor: "rgba(220, 38, 38, 0.04)",
                border: "1px solid rgba(220, 38, 38, 0.18)",
                color: "var(--color-error)",
              }}
            >
              <div role="alert">Failed to load role details for {selectedRoleID}.</div>
              <button
                type="button"
                className="mt-3 rounded-md px-3 py-2 text-sm font-medium"
                onClick={() => {
                  void selectedRoleDetails.refetch();
                }}
                style={{
                  border: "1px solid var(--color-error)",
                  backgroundColor: "transparent",
                  color: "var(--color-error)",
                  cursor: "pointer",
                }}
              >
                Retry
              </button>
            </div>
          </section>
        ) : (
          <RoleEditor
            role={isCreatingNew ? null : editorRole}
            resetToken={resetToken}
            isSaving={createRole.isPending || updateRole.isPending}
            isDeleting={deleteRole.isPending}
            onSave={handleSave}
            onDelete={handleDelete}
          />
        )}
      </div>
    </div>
  );
}
