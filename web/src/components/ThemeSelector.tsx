import { useTranslation } from 'react-i18next';
import { useSettingsStore } from '@/stores/settingsStore';
import { useState } from 'react';

/**
 * ThemeSelector Component
 *
 * Allows users to switch between light, dark, and system themes.
 * Applies theme immediately to prevent FOUC (Flash of Unstyled Content).
 *
 * @example
 * <ThemeSelector />
 */
export function ThemeSelector() {
  const { t } = useTranslation();
  const { settings, updateTheme, isLoading } = useSettingsStore();
  const [isSaving, setIsSaving] = useState(false);

  const handleThemeChange = async (theme: 'light' | 'dark' | 'system') => {
    setIsSaving(true);
    try {
      await updateTheme(theme);
    } catch (error) {
      console.error('Failed to update theme:', error);
    } finally {
      setIsSaving(false);
    }
  };

  if (!settings) return null;

  const themes: Array<{ value: 'light' | 'dark' | 'system'; icon: string }> = [
    { value: 'light', icon: '☀️' },
    { value: 'dark', icon: '🌙' },
    { value: 'system', icon: '💻' },
  ];

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          {t('settings.theme.title')}
        </h3>
        <p className="text-sm text-gray-600 dark:text-gray-400">
          {t('settings.theme.description')}
        </p>
      </div>

      <div className="grid grid-cols-3 gap-4">
        {themes.map(({ value, icon }) => (
          <button
            key={value}
            onClick={() => handleThemeChange(value)}
            disabled={isLoading || isSaving}
            className={`
              relative flex flex-col items-center gap-2 p-4 rounded-lg border-2 transition-all
              disabled:opacity-50 disabled:cursor-not-allowed
              ${
                settings.theme === value
                  ? 'border-primary-600 bg-primary-50 dark:bg-primary-950'
                  : 'border-gray-200 dark:border-gray-700 hover:border-primary-400'
              }
            `}
            aria-label={t(`settings.theme.${value}`)}
            aria-pressed={settings.theme === value}
          >
            <span className="text-3xl" role="img" aria-hidden="true">
              {icon}
            </span>
            <span className="text-sm font-medium text-gray-900 dark:text-gray-100">
              {t(`settings.theme.${value}`)}
            </span>
            {settings.theme === value && (
              <div className="absolute top-2 right-2">
                <svg
                  className="w-5 h-5 text-primary-600"
                  fill="currentColor"
                  viewBox="0 0 20 20"
                  aria-hidden="true"
                >
                  <path
                    fillRule="evenodd"
                    d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                    clipRule="evenodd"
                  />
                </svg>
              </div>
            )}
          </button>
        ))}
      </div>

      {isSaving && (
        <p className="text-sm text-primary-600 dark:text-primary-400">
          {t('settings.saving')}
        </p>
      )}
    </div>
  );
}
