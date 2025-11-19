import { useTranslation } from 'react-i18next';
import { useSettingsStore } from '@/stores/settingsStore';
import { useState } from 'react';

/**
 * LanguageSelector Component
 *
 * Allows users to select their preferred language.
 * Changes are applied immediately without page reload.
 *
 * @example
 * <LanguageSelector />
 */
export function LanguageSelector() {
  const { t, i18n } = useTranslation();
  const { settings, updateLanguage, isLoading } = useSettingsStore();
  const [isSaving, setIsSaving] = useState(false);

  const handleLanguageChange = async (language: string) => {
    setIsSaving(true);
    try {
      // Update i18n immediately for instant UI feedback
      await i18n.changeLanguage(language);
      localStorage.setItem('language', language);

      // Then sync with backend
      await updateLanguage(language);
    } catch (error) {
      console.error('Failed to update language:', error);
      // Revert i18n on error
      if (settings) {
        await i18n.changeLanguage(settings.language);
      }
    } finally {
      setIsSaving(false);
    }
  };

  if (!settings) return null;

  const languages = [
    { code: 'en', name: 'English', flag: '🇺🇸' },
    { code: 'ko', name: '한국어', flag: '🇰🇷' },
    { code: 'ja', name: '日本語', flag: '🇯🇵' },
  ];

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          {t('settings.language.title')}
        </h3>
        <p className="text-sm text-gray-600 dark:text-gray-400">
          {t('settings.language.description')}
        </p>
      </div>

      <div className="space-y-2">
        {languages.map(({ code, name, flag }) => (
          <button
            key={code}
            onClick={() => handleLanguageChange(code)}
            disabled={isLoading || isSaving}
            className={`
              w-full flex items-center justify-between p-4 rounded-lg border-2 transition-all
              disabled:opacity-50 disabled:cursor-not-allowed
              ${
                i18n.language === code
                  ? 'border-primary-600 bg-primary-50 dark:bg-primary-950'
                  : 'border-gray-200 dark:border-gray-700 hover:border-primary-400'
              }
            `}
            aria-label={name}
            aria-pressed={i18n.language === code}
          >
            <div className="flex items-center gap-3">
              <span className="text-2xl" role="img" aria-hidden="true">
                {flag}
              </span>
              <span className="text-base font-medium text-gray-900 dark:text-gray-100">
                {name}
              </span>
            </div>
            {i18n.language === code && (
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
