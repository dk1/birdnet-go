<!--
  NotificationTemplateEditor.svelte

  Purpose: Editable title/message template for one detection notification
  type (new species, lifer, ...). Extracted so NotificationsSettingsPage can
  render one instance per template type without duplicating the load/save/
  reset/test-fire logic. Each instance owns its own edited/saved state and
  reports whether it has unsaved changes via the bindable `hasChanges` prop
  so the parent page's tab-level "unsaved changes" indicator can aggregate
  across every template editor on the page.

  Props:
  - sectionTitle: string - already-translated heading text
  - templateKey: 'newSpecies' | 'lifer' - which sub-object of
    notification.templates this maps to, both for the initial load (done by
    the parent) and the PATCH body sent on save
  - initialTitle/initialMessage: string - the currently-saved template,
    fetched once by the parent before this component mounts
  - testEndpoint: string - POST target for "send test notification"
  - testFallbackSpecies: string - species name shown in the test-fire status
    message when the API response doesn't include one
  - hasChanges: boolean (bindable) - true when edited values differ from saved

  @component
-->
<script lang="ts">
  import { Bell } from '@lucide/svelte';
  import { t } from '$lib/i18n';
  import { api, ApiError } from '$lib/utils/api';
  import StatusBanner from '$lib/desktop/components/ui/StatusBanner.svelte';

  const STATUS_DISMISS_MS = 3000;
  const STATUS_DISMISS_ERROR_MS = 5000;
  const STATUS_DISMISS_LONG_MS = 10000;

  interface Props {
    sectionTitle: string;
    templateKey: 'newSpecies' | 'lifer';
    initialTitle: string;
    initialMessage: string;
    testEndpoint: string;
    testFallbackSpecies: string;
    hasChanges?: boolean;
  }

  let {
    sectionTitle,
    templateKey,
    initialTitle,
    initialMessage,
    testEndpoint,
    testFallbackSpecies,
    hasChanges = $bindable(false),
  }: Props = $props();

  let savedTitle = $state(initialTitle);
  let savedMessage = $state(initialMessage);
  let editedTitle = $state(initialTitle);
  let editedMessage = $state(initialMessage);

  let savingTemplate = $state(false);
  let templateStatusMessage = $state('');
  let templateStatusType = $state<'info' | 'success' | 'error'>('info');

  let generating = $state(false);
  let channelStatusMessage = $state('');
  let channelStatusType = $state<'info' | 'success' | 'error'>('info');

  let templateStatusTimeout: ReturnType<typeof setTimeout>;
  let channelStatusTimeout: ReturnType<typeof setTimeout>;

  $effect(() => {
    hasChanges = editedTitle !== savedTitle || editedMessage !== savedMessage;
  });

  async function saveTemplateConfig() {
    savingTemplate = true;
    templateStatusMessage = '';

    try {
      await api.patch('/api/v2/settings/notification', {
        templates: {
          [templateKey]: {
            title: editedTitle,
            message: editedMessage,
          },
        },
      });

      savedTitle = editedTitle;
      savedMessage = editedMessage;

      templateStatusMessage = t('settings.notifications.templates.saveSuccess');
      templateStatusType = 'success';

      clearTimeout(templateStatusTimeout);
      templateStatusTimeout = setTimeout(() => {
        templateStatusMessage = '';
      }, STATUS_DISMISS_MS);
    } catch (error) {
      templateStatusMessage = t('settings.notifications.templates.saveError', {
        message: (error as Error).message,
      });
      templateStatusType = 'error';

      clearTimeout(templateStatusTimeout);
      templateStatusTimeout = setTimeout(() => {
        templateStatusMessage = '';
      }, STATUS_DISMISS_ERROR_MS);
    } finally {
      savingTemplate = false;
    }
  }

  function resetTemplate() {
    const confirmReset = window.confirm(t('settings.notifications.templates.resetConfirm'));
    if (!confirmReset) return;
    editedTitle = '';
    editedMessage = '';
  }

  async function sendTestNotification() {
    if (hasChanges) {
      const confirmTest = window.confirm(
        t('settings.notifications.templates.unsavedChangesWarning')
      );
      if (!confirmTest) return;
    }

    generating = true;
    channelStatusMessage = '';
    channelStatusType = 'info';
    updateChannelStatus(
      t('settings.notifications.testNotification.statusMessages.sending'),
      'info'
    );

    try {
      interface TestNotificationResponse {
        title?: string;
      }
      const data = await api.post<TestNotificationResponse>(testEndpoint);
      generating = false;

      updateChannelStatus(
        t('settings.notifications.testNotification.statusMessages.success', {
          species: data.title || testFallbackSpecies,
        }),
        'success'
      );

      clearTimeout(channelStatusTimeout);
      channelStatusTimeout = setTimeout(() => {
        channelStatusMessage = '';
        channelStatusType = 'info';
      }, STATUS_DISMISS_ERROR_MS);
    } catch (error) {
      generating = false;
      if (error instanceof ApiError && error.status === 503) {
        updateChannelStatus(
          t('settings.notifications.testNotification.statusMessages.serviceUnavailable'),
          'error'
        );
      } else {
        updateChannelStatus(
          t('settings.notifications.testNotification.statusMessages.error', {
            message: (error as Error).message,
          }),
          'error'
        );
      }

      clearTimeout(channelStatusTimeout);
      channelStatusTimeout = setTimeout(() => {
        channelStatusMessage = '';
        channelStatusType = 'info';
      }, STATUS_DISMISS_LONG_MS);
    }
  }

  function updateChannelStatus(message: string, type: 'info' | 'success' | 'error') {
    channelStatusMessage = message;
    channelStatusType = type;
  }
</script>

<div class="rounded-lg bg-[var(--color-base-200)]">
  <div class="p-6">
    <h3 class="text-base font-semibold mb-4">{sectionTitle}</h3>

    <div class="space-y-4">
      <div>
        <label for="template-title-{templateKey}" class="block mb-1">
          <span class="text-sm font-semibold text-[var(--color-base-content)]"
            >{t('settings.notifications.templates.titleLabel')}</span
          >
        </label>
        <input
          id="template-title-{templateKey}"
          type="text"
          bind:value={editedTitle}
          class="w-full h-10 px-3 font-mono text-sm bg-[var(--color-base-100)] border border-[var(--color-base-300)] rounded-lg focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)] focus:border-transparent transition-colors"
          placeholder={t('settings.notifications.templates.titlePlaceholder')}
        />
      </div>

      <div>
        <label for="template-message-{templateKey}" class="block mb-1">
          <span class="text-sm font-semibold text-[var(--color-base-content)]"
            >{t('settings.notifications.templates.messageLabel')}</span
          >
        </label>
        <textarea
          id="template-message-{templateKey}"
          bind:value={editedMessage}
          class="w-full px-3 py-2 font-mono text-sm bg-[var(--color-base-100)] border border-[var(--color-base-300)] rounded-lg focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)] focus:border-transparent transition-colors resize-y"
          rows="6"
          placeholder={t('settings.notifications.templates.messagePlaceholder')}
        ></textarea>
      </div>

      {#if templateStatusMessage}
        <StatusBanner message={templateStatusMessage} type={templateStatusType} />
      {/if}

      {#if channelStatusMessage}
        <StatusBanner message={channelStatusMessage} type={channelStatusType} />
      {/if}

      <div class="flex gap-2 justify-end">
        <button
          onclick={resetTemplate}
          class="inline-flex items-center justify-center h-8 px-3 text-sm font-medium rounded-lg bg-transparent hover:bg-black/5 dark:hover:bg-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-base-content)] focus-visible:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          disabled={savingTemplate || generating}
        >
          {t('settings.notifications.templates.resetButton')}
        </button>
        <button
          onclick={saveTemplateConfig}
          class="inline-flex items-center justify-center gap-2 h-8 px-3 text-sm font-medium rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed {hasChanges
            ? 'bg-[var(--color-primary)] text-[var(--color-primary-content)] hover:opacity-90 focus-visible:ring-[var(--color-primary)]'
            : 'bg-transparent hover:bg-black/5 dark:hover:bg-white/10 focus-visible:ring-[var(--color-base-content)]'}"
          disabled={savingTemplate || generating || !hasChanges}
        >
          {#if savingTemplate}
            <span
              class="inline-block w-3 h-3 border-2 border-[var(--color-base-300)] border-t-current rounded-full animate-spin"
            ></span>
            <span>{t('settings.notifications.templates.savingButton')}</span>
          {:else}
            <span
              >{hasChanges
                ? t('settings.notifications.templates.saveButtonUnsaved')
                : t('settings.notifications.templates.saveButton')}</span
            >
          {/if}
        </button>
        <button
          onclick={sendTestNotification}
          disabled={generating || savingTemplate}
          class="inline-flex items-center justify-center gap-2 h-8 px-3 text-sm font-medium rounded-lg bg-[var(--color-secondary)] text-[var(--color-secondary-content)] hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-secondary)] focus-visible:ring-offset-2 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          title={hasChanges
            ? t('settings.notifications.templates.testWithUnsavedChanges')
            : t('settings.notifications.templates.testNormal')}
        >
          {#if generating}
            <span
              class="inline-block w-3 h-3 border-2 border-[var(--color-base-300)] border-t-current rounded-full animate-spin"
            ></span>
            <span>{t('settings.notifications.templates.sendingButton')}</span>
          {:else}
            <span class="flex items-center gap-1">
              <Bell class="size-4" />
              <span>{t('settings.notifications.templates.testButton')}</span>
            </span>
          {/if}
        </button>
      </div>
    </div>
  </div>
</div>
