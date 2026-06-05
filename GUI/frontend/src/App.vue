<script setup>
import { onMounted, ref } from 'vue'
import FloatingPanel from './components/FloatingPanel.vue'
import SettingsWindow from './components/SettingsWindow.vue'
import { EventsOn, GetStatus } from './api'

const status = ref({ state: 'Idle', message: 'Starting…' })
const settingsOpen = ref(false)

onMounted(async () => {
  try {
    status.value = await GetStatus()
  } catch (error) {
    status.value = { state: 'Error', message: String(error) }
  }
  EventsOn('runtime:status', (next) => {
    status.value = next
  })
  EventsOn('settings:open', () => {
    settingsOpen.value = true
  })
})
</script>

<template>
  <FloatingPanel :status="status" @open-settings="settingsOpen = true" />
  <SettingsWindow v-if="settingsOpen" :status="status" @close="settingsOpen = false" />
</template>
