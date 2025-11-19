import { useTranslation } from 'react-i18next';
import { useSettingsStore } from '@/stores/settingsStore';
import { useState } from 'react';
import type { RetentionOption } from '@/types/settings';

/**
 * DataRetentionSettings Component
 *
 * Configures automatic data deletion policies.
 * Displays retention options with user-friendly descriptions.
 *
 * @example
 * <DataRetentionSettings />
 */
export function DataRetentionSettings() {
  const { t } = useTranslation();
  const { dataRetention, updateDataRetention, isLoading } = useSettingsStore();
  const [isSaving, setIsSaving] = useState(false);

  const retentionOptions: RetentionOption[] = [
    {
      value: 30,
      label: t('settings.dataRetention.options.30days'),
      description: t('settings.dataRetention.options.30daysDesc'),
    },
    {
      value: 90,
      label: t('settings.dataRetention.options.90days'),
      description: t('settings.dataRetention.options.90daysDesc'),
    },
    {
      value: 365,
      label: t('settings.dataRetention.options.1year'),
      description: t('settings.dataRetention.options.1yearDesc'),
    },
    {
      value: null,
      label: t('settings.dataRetention.options.forever'),
      description: t('settings.dataRetention.options.foreverDesc'),
    },
  ];

  const handleToggleAutoDelete = async (enabled: boolean) => {
    if (!dataRetention) return;

    setIsSaving(true);
    try {
      await updateDataRetention(
        enabled,
        enabled ? (dataRetention.retention_days || 90) : undefined
      );
    } catch (error) {
      console.error('Failed to update auto delete:', error);
    } finally {
      setIsSaving(false);
    }
  };

  const handleRetentionChange = async (days: number | null) => {
    if (!dataRetention) return;

    setIsSaving(true);
    try {
      await updateDataRetention(days !== null, days || undefined);
    } catch (error) {
      console.error('Failed to update retention period:', error);
    } finally {
      setIsSaving(false);
    }
  };

  if (!dataRetention) return null;

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          {t('settings.dataRetention.title')}
        </h3>
        <p className="text-sm text-gray-600 dark:text-gray-400">
          {t('settings.dataRetention.description')}
        </p>
      </div>

      {/* Auto delete toggle */}
      <div className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
        <div>
          <p className="font-medium text-gray-900 dark:text-gray-100">
            {t('settings.dataRetention.autoDelete')}
          </p>
          <p className="text-sm text-gray-600 dark:text-gray-400">
            {t('settings.dataRetention.autoDeleteDescription')}
          </p>
        </div>
        <button
          onClick={() => handleToggleAutoDelete(!dataRetention.auto_delete_enabled)}
          disabled={isLoading || isSaving}
          className={`
            relative inline-flex h-6 w-11 items-center rounded-full transition-colors
            disabled:opacity-50 disabled:cursor-not-allowed
            ${dataRetention.auto_delete_enabled ? 'bg-primary-600' : 'bg-gray-300'}
          `}
          role="switch"
          aria-checked={dataRetention.auto_delete_enabled}
          aria-label={t('settings.dataRetention.autoDelete')}
        >
          <span
            className={`
              inline-block h-4 w-4 transform rounded-full bg-white transition-transform
              ${dataRetention.auto_delete_enabled ? 'translate-x-6' : 'translate-x-1'}
            `}
          />
        </button>
      </div>

      {/* Retention period options */}
      {dataRetention.auto_delete_enabled && (
        <div className="space-y-3">
          <label className="block text-sm font-medium text-gray-900 dark:text-gray-100">
            {t('settings.dataRetention.retentionPeriod')}
          </label>
          <div className="space-y-2">
            {retentionOptions.map((option) => (
              <button
                key={option.value ?? 'forever'}
                onClick={() => handleRetentionChange(option.value)}
                disabled={isLoading || isSaving}
                className={`
                  w-full text-left p-4 rounded-lg border-2 transition-all
                  disabled:opacity-50 disabled:cursor-not-allowed
                  ${
                    dataRetention.retention_days === option.value
                      ? 'border-primary-600 bg-primary-50 dark:bg-primary-950'
                      : 'border-gray-200 dark:border-gray-700 hover:border-primary-400'
                  }
                `}
                aria-pressed={dataRetention.retention_days === option.value}
              >
                <div className="flex items-start justify-between">
                  <div>
                    <p className="font-medium text-gray-900 dark:text-gray-100">
                      {option.label}
                    </p>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      {option.description}
                    </p>
                  </div>
                  {dataRetention.retention_days === option.value && (
                    <svg
                      className="w-5 h-5 text-primary-600 flex-shrink-0"
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
                  )}
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {isSaving && (
        <p className="text-sm text-primary-600 dark:text-primary-400">
          {t('settings.saving')}
        </p>
      )}
    </div>
  );
}
