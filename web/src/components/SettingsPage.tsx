import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useSettingsStore } from '@/stores/settingsStore';
import { ThemeSelector } from './ThemeSelector';
import { LanguageSelector } from './LanguageSelector';
import { DataRetentionSettings } from './DataRetentionSettings';

/**
 * SettingsPage Component
 *
 * Main settings page that combines all setting sections.
 * Fetches user settings on mount and provides organized sections.
 *
 * Accessibility features:
 * - Semantic HTML with proper headings
 * - ARIA labels for interactive elements
 * - Keyboard navigation support
 * - Screen reader friendly error messages
 *
 * Performance optimizations:
 * - Uses Zustand for efficient state management
 * - Optimistic UI updates for theme changes
 * - Lazy loading of settings data
 *
 * @example
 * <SettingsPage />
 */
export function SettingsPage() {
  const { t } = useTranslation();
  const { fetchAllSettings, isLoading, error } = useSettingsStore();

  useEffect(() => {
    fetchAllSettings();
  }, [fetchAllSettings]);

  if (isLoading && !useSettingsStore.getState().settings) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-white dark:bg-gray-900">
        <div className="text-center space-y-4">
          <div
            className="inline-block h-12 w-12 animate-spin rounded-full border-4 border-solid border-primary-600 border-r-transparent"
            role="status"
            aria-label="Loading settings"
          />
          <p className="text-gray-600 dark:text-gray-400">Loading settings...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div
        className="flex items-center justify-center min-h-screen bg-white dark:bg-gray-900"
        role="alert"
        aria-live="assertive"
      >
        <div className="text-center space-y-4 p-6 bg-red-50 dark:bg-red-950 rounded-lg border-2 border-red-200 dark:border-red-800">
          <svg
            className="mx-auto h-12 w-12 text-red-600"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            aria-hidden="true"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
          <p className="text-red-900 dark:text-red-100 font-semibold">
            {t('settings.error')}
          </p>
          <p className="text-sm text-red-700 dark:text-red-300">{error}</p>
          <button
            onClick={() => fetchAllSettings()}
            className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 transition-colors">
      <div className="max-w-4xl mx-auto px-4 py-8 sm:px-6 lg:px-8">
        {/* Header */}
        <header className="mb-8">
          <h1 className="text-3xl font-bold text-gray-900 dark:text-gray-100">
            {t('settings.title')}
          </h1>
        </header>

        {/* Settings sections */}
        <div className="space-y-8">
          {/* Theme section */}
          <section
            className="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6 transition-colors"
            aria-labelledby="theme-heading"
          >
            <ThemeSelector />
          </section>

          {/* Language section */}
          <section
            className="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6 transition-colors"
            aria-labelledby="language-heading"
          >
            <LanguageSelector />
          </section>

          {/* Data retention section */}
          <section
            className="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6 transition-colors"
            aria-labelledby="data-retention-heading"
          >
            <DataRetentionSettings />
          </section>
        </div>

        {/* Footer */}
        <footer className="mt-12 text-center text-sm text-gray-500 dark:text-gray-400">
          <p>
            Donelist &copy; {new Date().getFullYear()} - Done tracking, not Todo planning
          </p>
        </footer>
      </div>
    </div>
  );
}
