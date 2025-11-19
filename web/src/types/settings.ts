// Backend API types matching the Go models

export interface Profile {
  id: string;
  user_id: string;
  bio?: string;
  avatar_url?: string;
  timezone: string;
  timezone_auto_detected: boolean;
  locale: string;
  created_at: string;
  updated_at: string;
  version: number;
}

export interface Settings {
  id: string;
  user_id: string;
  theme: 'light' | 'dark' | 'system';
  language: string;
  date_format: string;
  time_format: '12h' | '24h';
  week_start_day: number;
  created_at: string;
  updated_at: string;
  version: number;
}

export interface NotificationSettings {
  id: string;
  user_id: string;
  email_notifications: boolean;
  push_notifications: boolean;
  checkin_reminders: boolean;
  reminder_interval_minutes: number;
  dnd_enabled: boolean;
  dnd_start_time?: string;
  dnd_end_time?: string;
  dnd_days: number[];
  created_at: string;
  updated_at: string;
  version: number;
}

export interface DataRetentionSettings {
  id: string;
  user_id: string;
  auto_delete_enabled: boolean;
  retention_days?: number;
  delete_after_inactivity_days?: number;
  last_activity_at: string;
  created_at: string;
  updated_at: string;
  version: number;
}

export interface ProfileWithSettings {
  profile: Profile;
  settings: Settings;
  notification_settings: NotificationSettings;
  data_retention_settings: DataRetentionSettings;
}

// Update input types
export interface UpdateSettingsInput {
  theme?: 'light' | 'dark' | 'system';
  language?: string;
  date_format?: string;
  time_format?: '12h' | '24h';
  week_start_day?: number;
  version: number;
}

export interface UpdateDataRetentionSettingsInput {
  auto_delete_enabled?: boolean;
  retention_days?: number;
  delete_after_inactivity_days?: number;
  version: number;
}

// UI-specific types
export interface RetentionOption {
  value: number | null;
  label: string;
  description: string;
}
