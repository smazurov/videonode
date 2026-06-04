<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'
import PipelineDiagram from './PipelineDiagram.vue'

const { Layout } = DefaultTheme
</script>

<template>
  <Layout>
    <template #home-hero-image>
      <PipelineDiagram />
    </template>
  </Layout>
</template>
