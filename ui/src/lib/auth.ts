const AUTH_STORAGE_KEY = 'auth_credentials';

export function getAuthCredentials(): string | null {
  return localStorage.getItem(AUTH_STORAGE_KEY);
}

export function setAuthCredentials(username: string, password: string): void {
  localStorage.setItem(AUTH_STORAGE_KEY, btoa(`${username}:${password}`));
}

export function clearAuthCredentials(): void {
  localStorage.removeItem(AUTH_STORAGE_KEY);
}
