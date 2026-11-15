
<script lang="ts">
  import { onMount } from 'svelte';
  import { API_BASE_URL } from '../config';
  import { apiFetch } from '../api';

  interface Permissions {
    upload: boolean;
    edit: boolean;
    delete: boolean;
  }

  interface User {
    username: string;
    admin: boolean;
    disabled: boolean;
    permissions: Permissions;
  }

  let users: User[] = [];
  let loading = true;
  let creating = false;
  let error = '';
  let success = '';

  let username = '';
  let password = '';
  let newPermissions: Permissions = {
    upload: false,
    edit: false,
    delete: false
  };

  let updatingUsers = new Set<string>();
  let deletingUsers = new Set<string>();

  function clearMessages() {
    error = '';
    success = '';
  }

  async function responseError(response: Response): Promise<string> {
    try {
      const body = await response.json();
      return body.error || body.message || `Request failed (${response.status})`;
    } catch {
      return `Request failed (${response.status})`;
    }
  }

  async function loadUsers() {
    loading = true;
    clearMessages();

    try {
      const response = await apiFetch(`${API_BASE_URL}/auth/users`);

      if (!response.ok) {
        throw new Error(await responseError(response));
      }

      const body = await response.json();
      users = (body.users || []).map((user: User) => ({
        ...user,
        permissions: {
          upload: user.admin || user.permissions?.upload === true,
          edit: user.admin || user.permissions?.edit === true,
          delete: user.admin || user.permissions?.delete === true
        }
      }));
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to load users';
    } finally {
      loading = false;
    }
  }

  async function createUser() {
    clearMessages();

    if (!username.trim() || !password) {
      error = 'Username and password are required.';
      return;
    }

    creating = true;

    try {
      const response = await apiFetch(`${API_BASE_URL}/auth/users`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: username.trim(),
          password,
          permissions: { ...newPermissions }
        })
      });

      if (!response.ok) {
        throw new Error(await responseError(response));
      }

      username = '';
      password = '';
      newPermissions = { upload: false, edit: false, delete: false };
      await loadUsers();
      success = 'User created.';
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to create user';
    } finally {
      creating = false;
    }
  }

  async function updateUser(user: User, changes: {
    permissions?: Permissions;
    disabled?: boolean;
  }) {
    clearMessages();
    updatingUsers = new Set(updatingUsers).add(user.username);

    try {
      const response = await apiFetch(
        `${API_BASE_URL}/auth/users/${encodeURIComponent(user.username)}`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            permissions: changes.permissions ?? user.permissions,
            disabled: changes.disabled ?? user.disabled
          })
        }
      );

      if (!response.ok) {
        throw new Error(await responseError(response));
      }

      await loadUsers();
      success = `Updated ${user.username}.`;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to update user';
    } finally {
      const next = new Set(updatingUsers);
      next.delete(user.username);
      updatingUsers = next;
    }
  }

  async function setPermission(
    user: User,
    permission: keyof Permissions,
    enabled: boolean
  ) {
    await updateUser(user, {
      permissions: {
        ...user.permissions,
        [permission]: enabled
      }
    });
  }

  async function setDisabled(user: User, disabled: boolean) {
    await updateUser(user, { disabled });
  }

  async function deleteUser(user: User) {
    if (!confirm(`Permanently delete the account "${user.username}"?`)) {
      return;
    }

    clearMessages();
    deletingUsers = new Set(deletingUsers).add(user.username);

    try {
      const response = await apiFetch(
        `${API_BASE_URL}/auth/users/${encodeURIComponent(user.username)}`,
        { method: 'DELETE' }
      );

      if (!response.ok) {
        throw new Error(await responseError(response));
      }

      await loadUsers();
      success = `Deleted ${user.username}.`;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unable to delete user';
    } finally {
      const next = new Set(deletingUsers);
      next.delete(user.username);
      deletingUsers = next;
    }
  }

  onMount(loadUsers);
</script>

<section class="users-page">
  <header class="page-header">
    <div>
      <h2>User management</h2>
      <p>Manage accounts and their access to the shared library.</p>
    </div>

    <button class="secondary-button" on:click={loadUsers} disabled={loading}>
      Refresh
    </button>
  </header>

  {#if error}
    <div class="message error" role="alert">{error}</div>
  {/if}

  {#if success}
    <div class="message success" role="status">{success}</div>
  {/if}

  <section class="panel create-panel">
    <h3>Create account</h3>
    <p class="panel-description">
      New accounts are read-only unless you explicitly grant permissions.
    </p>

    <form on:submit|preventDefault={createUser}>
      <div class="form-fields">
        <label>
          Username
          <input
            bind:value={username}
            autocomplete="off"
            required
            disabled={creating}
          />
        </label>

        <label>
          Password
          <input
            type="password"
            bind:value={password}
            autocomplete="new-password"
            required
            disabled={creating}
          />
        </label>
      </div>

      <fieldset>
        <legend>Permissions</legend>

        <div class="permission-options">
          <label>
            <input type="checkbox" bind:checked={newPermissions.upload} />
            Upload
          </label>

          <label>
            <input type="checkbox" bind:checked={newPermissions.edit} />
            Edit
          </label>

          <label>
            <input type="checkbox" bind:checked={newPermissions.delete} />
            Delete
          </label>
        </div>
      </fieldset>

      <button class="primary-button" type="submit" disabled={creating}>
        {creating ? 'Creating…' : 'Create user'}
      </button>
    </form>
  </section>

  <section class="panel accounts-panel">
    <div class="section-heading">
      <div>
        <h3>Accounts</h3>
        <p class="panel-description">
          Permissions apply to the shared library, not individual items.
        </p>
      </div>
      <span class="account-count">{users.length} accounts</span>
    </div>

    {#if loading}
      <p class="empty-state">Loading accounts…</p>
    {:else if users.length === 0}
      <p class="empty-state">No accounts found.</p>
    {:else}
      <div class="table-wrapper">
        <table>
          <thead>
            <tr>
              <th scope="col">Account</th>
              <th scope="col">Upload</th>
              <th scope="col">Edit</th>
              <th scope="col">Delete</th>
              <th scope="col">Status</th>
              <th scope="col">Actions</th>
            </tr>
          </thead>

          <tbody>
            {#each users as user (user.username)}
              <tr class:disabled-row={user.disabled}>
                <th scope="row">
                  <span class="username">{user.username}</span>
                  {#if user.admin}
                    <span class="role-label">Administrator</span>
                  {/if}
                </th>

                {#each ['upload', 'edit', 'delete'] as permission}
                  <td>
                    <input
                      type="checkbox"
                      aria-label={`${permission} permission for ${user.username}`}
                      checked={user.permissions[permission as keyof Permissions]}
                      disabled={user.admin || user.disabled || updatingUsers.has(user.username)}
                      on:change={(event) =>
                        setPermission(
                          user,
                          permission as keyof Permissions,
                          event.currentTarget.checked
                        )}
                    />
                  </td>
                {/each}

                <td>
                  <label class="status-control">
                    <input
                      type="checkbox"
                      checked={!user.disabled}
                      aria-label={`Account enabled for ${user.username}`}
                      disabled={user.admin || updatingUsers.has(user.username)}
                      on:change={(event) =>
                        setDisabled(user, !event.currentTarget.checked)}
                    />
                    <span>{user.disabled ? 'Disabled' : 'Enabled'}</span>
                  </label>
                </td>

                <td>
                  <button
                    class="danger-button"
                    type="button"
                    disabled={user.admin || deletingUsers.has(user.username) || updatingUsers.has(user.username)}
                    on:click={() => deleteUser(user)}
                  >
                    {deletingUsers.has(user.username) ? 'Deleting…' : 'Delete account'}
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
</section>

<style>
  .users-page {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
    max-width: 1100px;
  }

  .page-header,
  .section-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  h2, h3, p {
    margin-top: 0;
  }

  h2 {
    margin-bottom: 0.35rem;
  }

  h3 {
    margin-bottom: 0.4rem;
  }

  .page-header p,
  .panel-description {
    color: var(--text-secondary, #888);
    margin-bottom: 0;
  }

  .panel {
    border: 1px solid var(--border-color, #333);
    border-radius: 8px;
    padding: 1.25rem;
    background: var(--panel-background, transparent);
  }

  .create-panel form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    margin-top: 1.25rem;
  }

  .form-fields {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
  }

  .form-fields label {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    font-size: 0.9rem;
  }

  input:not([type="checkbox"]) {
    width: 100%;
    box-sizing: border-box;
    padding: 0.65rem 0.75rem;
    color: inherit;
    background: var(--input-background, #222);
    border: 1px solid var(--border-color, #444);
    border-radius: 5px;
  }

  fieldset {
    border: 1px solid var(--border-color, #444);
    border-radius: 5px;
    padding: 0.75rem 1rem 1rem;
  }

  legend {
    padding: 0 0.35rem;
    color: var(--text-secondary, #aaa);
  }

  .permission-options {
    display: flex;
    flex-wrap: wrap;
    gap: 1.5rem;
  }

  .permission-options label,
  .status-control {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
  }

  button {
    border: 0;
    border-radius: 5px;
    padding: 0.6rem 0.9rem;
    cursor: pointer;
    font: inherit;
  }

  button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .primary-button {
    align-self: flex-start;
    color: white;
    background: var(--accent-color, #4b72c2);
  }

  .secondary-button {
    color: inherit;
    background: var(--button-background, #333);
  }

  .danger-button {
    color: white;
    background: #a33b3b;
    white-space: nowrap;
  }

  .account-count,
  .role-label {
    color: var(--text-secondary, #999);
    font-size: 0.8rem;
  }

  .role-label {
    display: block;
    margin-top: 0.2rem;
  }

  .table-wrapper {
    overflow-x: auto;
    margin-top: 1rem;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    text-align: left;
  }

  th, td {
    padding: 0.85rem 0.75rem;
    border-bottom: 1px solid var(--border-color, #333);
    vertical-align: middle;
  }

  thead th {
    color: var(--text-secondary, #aaa);
    font-size: 0.8rem;
    font-weight: 600;
  }

  tbody th {
    font-weight: 500;
  }

  .disabled-row {
    opacity: 0.55;
  }

  .empty-state {
    padding: 1.5rem 0;
    color: var(--text-secondary, #999);
  }

  .message {
    padding: 0.8rem 1rem;
    border-radius: 5px;
  }

  .message.error {
    color: #ffb5b5;
    background: rgba(180, 45, 45, 0.15);
    border: 1px solid rgba(180, 45, 45, 0.4);
  }

  .message.success {
    color: #a8dfb2;
    background: rgba(45, 140, 65, 0.12);
    border: 1px solid rgba(45, 140, 65, 0.35);
  }

  @media (max-width: 650px) {
    .form-fields {
      grid-template-columns: 1fr;
    }

    .page-header,
    .section-heading {
      align-items: flex-start;
    }
  }
</style>

