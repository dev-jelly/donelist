import { create } from 'zustand';
import type {
  Settings,
  DataRetentionSettings,
  ProfileWithSettings,
} from '@/types/settings';
import { settingsApi } from '@/lib/api';

interface SettingsState {
  settings: Settings | null;
  dataRetention: DataRetentionSettings | null;
  isLoading: boolean;
  error: string | null;

  // Actions
  fetchAllSettings: () => Promise<void>;
  updateTheme: (theme: 'light' | 'dark' | 'system') => Promise<void>;
  updateLanguage: (language: string) => Promise<void>;
  updateDataRetention: (
    autoDeleteEnabled: boolean,
    retentionDays?: number | null
  ) => Promise<void>;
  applyTheme: (theme: 'light' | 'dark' | 'system') => void;
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  settings: null,
  dataRetention: null,
  isLoading: false,
  error: null,

  fetchAllSettings: async () => {
    set({ isLoading: true, error: null });
    try {
      const data: ProfileWithSettings = await settingsApi.getAll();
      set({
        settings: data.settings,
        dataRetention: data.data_retention_settings,
        isLoading: false,
      });

      // Apply theme immediately
      get().applyTheme(data.settings.theme);
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Failed to fetch settings',
        isLoading: false,
      });
    }
  },

  updateTheme: async (theme: 'light' | 'dark' | 'system') => {
    const { settings } = get();
    if (!settings) return;

    set({ isLoading: true, error: null });
    try {
      const updated = await settingsApi.updateSettings({
        theme,
        version: settings.version,
      });
      set({ settings: updated, isLoading: false });

      // Apply theme to DOM
      get().applyTheme(theme);
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Failed to update theme',
        isLoading: false,
      });
      throw error;
    }
  },

  updateLanguage: async (language: string) => {
    const { settings } = get();
    if (!settings) return;

    set({ isLoading: true, error: null });
    try {
      const updated = await settingsApi.updateSettings({
        language,
        version: settings.version,
      });
      set({ settings: updated, isLoading: false });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Failed to update language',
        isLoading: false,
      });
      throw error;
    }
  },

  updateDataRetention: async (
    autoDeleteEnabled: boolean,
    retentionDays?: number | null
  ) => {
    const { dataRetention } = get();
    if (!dataRetention) return;

    set({ isLoading: true, error: null });
    try {
      const updated = await settingsApi.updateDataRetention({
        auto_delete_enabled: autoDeleteEnabled,
        retention_days: retentionDays === null ? undefined : retentionDays,
        version: dataRetention.version,
      });
      set({ dataRetention: updated, isLoading: false });
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : 'Failed to update data retention',
        isLoading: false,
      });
      throw error;
    }
  },

  applyTheme: (theme: 'light' | 'dark' | 'system') => {
    const root = document.documentElement;

    // Remove existing theme classes
    root.classList.remove('light', 'dark');

    if (theme === 'system') {
      // Use system preference
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      root.classList.add(prefersDark ? 'dark' : 'light');
    } else {
      root.classList.add(theme);
    }
  },
}));
