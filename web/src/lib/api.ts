// API client for backend communication
import type {
  Settings,
  DataRetentionSettings,
  ProfileWithSettings,
  UpdateSettingsInput,
  UpdateDataRetentionSettingsInput,
} from '@/types/settings';

const API_BASE_URL = import.meta.env.VITE_API_URL || '/api';

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

async function fetchWithAuth<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const token = localStorage.getItem('auth_token');

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token && { Authorization: `Bearer ${token}` }),
      ...options.headers,
    },
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Unknown error' }));
    throw new ApiError(response.status, error.error || 'Request failed');
  }

  return response.json();
}

export const settingsApi = {
  // Get all settings
  getAll: () => fetchWithAuth<ProfileWithSettings>('/profile/all'),

  // Settings endpoints
  getSettings: () => fetchWithAuth<Settings>('/settings'),

  updateSettings: (input: UpdateSettingsInput) =>
    fetchWithAuth<Settings>('/settings', {
      method: 'PATCH',
      body: JSON.stringify(input),
    }),

  // Data retention endpoints
  getDataRetention: () =>
    fetchWithAuth<DataRetentionSettings>('/settings/data-retention'),

  updateDataRetention: (input: UpdateDataRetentionSettingsInput) =>
    fetchWithAuth<DataRetentionSettings>('/settings/data-retention', {
      method: 'PATCH',
      body: JSON.stringify(input),
    }),
};

export { ApiError };
