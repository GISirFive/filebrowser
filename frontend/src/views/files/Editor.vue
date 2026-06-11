<template>
  <div id="editor-container">
    <header-bar>
      <action icon="close" :label="t('buttons.close')" @action="close()" />
      <title>{{ fileStore.req?.name ?? "" }}</title>

      <action
        icon="add"
        @action="increaseFontSize"
        :label="t('buttons.increaseFontSize')"
      />
      <span class="editor-font-size">{{ fontSize }}px</span>
      <action
        icon="remove"
        @action="decreaseFontSize"
        :label="t('buttons.decreaseFontSize')"
      />

      <action
        v-if="authStore.user?.perm.modify"
        id="save-button"
        icon="save"
        :label="t('buttons.save')"
        @action="save()"
      />

      <action
        v-if="isMarkdownFile"
        icon="preview"
        :label="modeToggleLabel"
        @action="toggleMode()"
      />
    </header-bar>

    <!-- preview container -->
    <div class="loading delayed" v-if="layoutStore.loading">
      <div class="spinner">
        <div class="bounce1"></div>
        <div class="bounce2"></div>
        <div class="bounce3"></div>
      </div>
    </div>
    <template v-else>
      <div class="editor-header">
        <Breadcrumbs base="/files" noLink />

        <div>
          <button
            :disabled="isSelectionEmpty"
            @click="executeEditorCommand('copy')"
          >
            <span><i class="material-icons">content_copy</i></span>
          </button>
          <button
            :disabled="isSelectionEmpty"
            @click="executeEditorCommand('cut')"
          >
            <span><i class="material-icons">content_cut</i></span>
          </button>
          <button @click="executeEditorCommand('paste')">
            <span><i class="material-icons">content_paste</i></span>
          </button>
          <button @click="executeEditorCommand('openCommandPalette')">
            <span><i class="material-icons">more_vert</i></span>
          </button>
        </div>
      </div>

      <div
        v-show="isMarkdownFile"
        id="cherry-container"
      ></div>
      <form v-show="!isMarkdownFile" id="editor"></form>
    </template>
  </div>
</template>

<script setup lang="ts">
import { files as api } from "@/api";
import buttons from "@/utils/buttons";
import url from "@/utils/url";
import ace, { Ace, version as ace_version } from "ace-builds";
import "ace-builds/src-noconflict/ext-language_tools";
import modelist from "ace-builds/src-noconflict/ext-modelist";
import Cherry from "cherry-markdown";
import "cherry-markdown/dist/cherry-markdown.css";

import Breadcrumbs from "@/components/Breadcrumbs.vue";
import Action from "@/components/header/Action.vue";
import HeaderBar from "@/components/header/HeaderBar.vue";
import { useAuthStore } from "@/stores/auth";
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";
import { getEditorTheme, getTheme } from "@/utils/theme";
import { computed, inject, onBeforeUnmount, onMounted, ref, watchEffect } from "vue";
import { useI18n } from "vue-i18n";
import { onBeforeRouteUpdate, useRoute, useRouter } from "vue-router";
import { read, copy } from "@/utils/clipboard";

const $showError = inject<IToastError>("$showError")!;

const fileStore = useFileStore();
const authStore = useAuthStore();
const layoutStore = useLayoutStore();

const { t } = useI18n();

const route = useRoute();
const router = useRouter();

const editor = ref<Ace.Editor | null>(null);
const cherryInstance = ref<any>(null);
const fontSize = ref(parseInt(localStorage.getItem("editorFontSize") || "14"));
const initialContent = ref("");

const editorMode = ref<"edit&preview" | "previewOnly">("previewOnly");
const themeObserver = ref<MutationObserver | null>(null);

const resolveCherryTheme = () => {
  const current = getTheme();
  const isDark = current === "dark";
  return {
    themeList: [
      { className: "default", label: "Default" },
      { className: "dark", label: "Dark" },
    ],
    mainTheme: isDark ? "dark" : "default",
    codeBlockTheme: isDark ? "monokai" : "default",
    inlineCodeTheme: isDark ? "black" as const : "red" as const,
  };
};

const syncCherryTheme = () => {
  if (!cherryInstance.value) return;
  const wrapper = document.querySelector<HTMLElement>(
    "#cherry-container .cherry"
  );
  if (!wrapper) return;
  const { mainTheme } = resolveCherryTheme();
  const toRemove: string[] = [];
  wrapper.classList.forEach((c) => {
    if (c.startsWith("theme__")) toRemove.push(c);
  });
  toRemove.forEach((c) => wrapper.classList.remove(c));
  wrapper.classList.add(`theme__${mainTheme}`);
};

const startThemeObserver = () => {
  stopThemeObserver();
  themeObserver.value = new MutationObserver((mutations) => {
    for (const m of mutations) {
      if (m.attributeName === "class") {
        syncCherryTheme();
        return;
      }
    }
  });
  themeObserver.value.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["class"],
  });
};

const stopThemeObserver = () => {
  if (themeObserver.value) {
    themeObserver.value.disconnect();
    themeObserver.value = null;
  }
};
const isMarkdownFile =
  fileStore.req?.name.endsWith(".md") ||
  fileStore.req?.name.endsWith(".markdown");

const modeToggleLabel = computed(() => {
  return editorMode.value === "previewOnly"
    ? t("buttons.editAsText")
    : t("buttons.preview");
});

const isSelectionEmpty = ref(true);

const executeEditorCommand = (name: string) => {
  if (name == "paste") {
    read()
      .then((data) => {
        editor.value?.execCommand("paste", {
          text: data,
        });
      })
      .catch((e) => {
        if (
          document.queryCommandSupported &&
          document.queryCommandSupported("paste")
        ) {
          document.execCommand("paste");
        } else {
          console.warn("the clipboard api is not supported", e);
        }
      });
    return;
  }
  if (name == "copy" || name == "cut") {
    const selectedText = editor.value?.getCopyText();
    copy({ text: selectedText });
  }
  editor.value?.execCommand(name);
};

const toggleMode = () => {
  if (!cherryInstance.value) return;

  const nextMode =
    editorMode.value === "previewOnly" ? "edit&preview" : "previewOnly";
  editorMode.value = nextMode;
  cherryInstance.value.switchModel(nextMode);
  setTimeout(updateFontSize, 0);
};

const initCherry = (content: string) => {
  destroyCherry();
  initialContent.value = content;
  cherryInstance.value = new Cherry({
    id: "cherry-container",
    value: content,
    editor: { defaultModel: editorMode.value },
    toolbars: {
      showToolbar: true,
      toc: { defaultModel: "full", updateLocationHash: false },
    },
    themeSettings: resolveCherryTheme(),
  });
  startThemeObserver();
  // Cherry may normalize the content (e.g. trim trailing newlines,
  // normalize whitespace). Snap the actual value so isClean() compares
  // against what Cherry really holds, not the raw server response.
  setTimeout(() => {
    if (cherryInstance.value) {
      initialContent.value = cherryInstance.value.getValue();
    }
  }, 0);
  setTimeout(updateFontSize, 0);
};

const destroyCherry = () => {
  stopThemeObserver();
  if (cherryInstance.value) {
    try {
      cherryInstance.value.destroy();
    } catch (e) {}
    cherryInstance.value = null;
  }
};

const handleCherryClick = (e: MouseEvent) => {
  const target = e.target as HTMLElement;
  const anchor = target.closest("a[href]");
  if (!anchor) return;
  const href = anchor.getAttribute("href");
  if (!href || !href.startsWith("#")) return;
  e.preventDefault();
  const id = href.slice(1);
  const el = document.getElementById(id);
  if (el) {
    el.scrollIntoView({ behavior: "smooth" });
  }
};

onMounted(() => {
  window.addEventListener("keydown", keyEvent);
  window.addEventListener("beforeunload", handlePageChange);

  const cherryContainer = document.getElementById("cherry-container");
  cherryContainer?.addEventListener("click", handleCherryClick);

  const fileContent = fileStore.req?.content || "";

  ace.config.set(
    "basePath",
    `https://cdn.jsdelivr.net/npm/ace-builds@${ace_version}/src-min-noconflict/`
  );

  if (!layoutStore.loading) {
    initEditor(fileContent);
  } else {
    const unwatch = watchEffect(() => {
      if (!layoutStore.loading) {
        setTimeout(() => {
          initEditor(fileContent);
          unwatch();
        }, 50);
      }
    });
  }
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", keyEvent);
  window.removeEventListener("beforeunload", handlePageChange);
  document
    .getElementById("cherry-container")
    ?.removeEventListener("click", handleCherryClick);
  destroyCherry();
  editor.value?.destroy();
});

onBeforeRouteUpdate((to, from, next) => {
  if (isClean()) {
    next();
    return;
  }

  layoutStore.showHover({
    prompt: "discardEditorChanges",
    confirm: (event: Event) => {
      event.preventDefault();
      next();
    },
    saveAction: async () => {
      await save();
      next();
    },
  });
});

const initEditor = (fileContent: string) => {
  if (isMarkdownFile) {
    initCherry(fileContent);
  } else {
    initAceEditor(fileContent);
  }
};

const initAceEditor = (fileContent: string) => {
  editor.value = ace.edit("editor", {
    value: fileContent,
    showPrintMargin: false,
    readOnly: fileStore.req?.type === "textImmutable",
    theme: getEditorTheme(authStore.user?.aceEditorTheme ?? ""),
    mode: modelist.getModeForPath(fileStore.req!.name).mode,
    wrap: true,
    enableBasicAutocompletion: true,
    enableLiveAutocompletion: true,
    enableSnippets: true,
  });

  editor.value.setFontSize(fontSize.value);
  editor.value.focus();

  const selection = editor.value?.getSelection();
  selection.on("changeSelection", function () {
    isSelectionEmpty.value = selection.isEmpty();
  });
};

const keyEvent = (event: KeyboardEvent) => {
  if (event.code === "Escape") {
    close();
  }

  if (!event.ctrlKey && !event.metaKey) {
    return;
  }

  if (event.key !== "s") {
    return;
  }

  event.preventDefault();
  save();
};

const handlePageChange = (event: BeforeUnloadEvent) => {
  if (!isClean()) {
    event.preventDefault();
    event.returnValue = true;
  }
};

const getContent = () => {
  if (isMarkdownFile && cherryInstance.value) {
    return cherryInstance.value.getValue();
  }
  return editor.value?.getValue() || "";
};

const markClean = () => {
  if (isMarkdownFile && cherryInstance.value) {
    initialContent.value = cherryInstance.value.getValue();
  } else {
    editor.value?.session.getUndoManager().markClean();
  }
};

const isClean = () => {
  if (isMarkdownFile && cherryInstance.value) {
    return cherryInstance.value.getValue() === initialContent.value;
  }
  return editor.value?.session.getUndoManager().isClean() ?? true;
};

const save = async (throwError?: boolean) => {
  const button = "save";
  buttons.loading("save");

  try {
    await api.put(route.path, getContent());
    markClean();
    buttons.success(button);
  } catch (e: any) {
    buttons.done(button);
    $showError(e);
    if (throwError) throw e;
  }
};

const updateFontSize = () => {
  const previewer = document.querySelector(
    "#cherry-container .cherry-previewer"
  ) as HTMLElement | null;
  if (previewer) {
    previewer.style.fontSize = fontSize.value + "px";
  }
  const codeMirror = document.querySelector(
    "#cherry-container .cherry-editor .CodeMirror"
  ) as HTMLElement | null;
  if (codeMirror) {
    codeMirror.style.fontSize = fontSize.value + "px";
  }
};

const increaseFontSize = () => {
  fontSize.value += 1;
  editor.value?.setFontSize(fontSize.value);
  updateFontSize();
  localStorage.setItem("editorFontSize", fontSize.value.toString());
};

const decreaseFontSize = () => {
  if (fontSize.value > 1) {
    fontSize.value -= 1;
    editor.value?.setFontSize(fontSize.value);
    updateFontSize();
    localStorage.setItem("editorFontSize", fontSize.value.toString());
  }
};

const close = () => {
  if (!isClean()) {
    layoutStore.showHover({
      prompt: "discardEditorChanges",
      confirm: (event: Event) => {
        event.preventDefault();
        if (isMarkdownFile && cherryInstance.value) {
          cherryInstance.value.setValue(initialContent.value);
        } else {
          editor.value?.session.getUndoManager().reset();
        }
        finishClose();
      },
      saveAction: async () => {
        try {
          await save(true);
          finishClose();
        } catch {}
      },
    });
    return;
  }
  finishClose();
};

const finishClose = () => {
  const uri = url.removeLastDir(route.path) + "/";
  router.push({ path: uri });
};
</script>

<style scoped>
.editor-font-size {
  margin: 0 0.5em;
  color: var(--fg);
}

.editor-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.editor-header > div > button {
  background: transparent;
  color: var(--action);
  border: none;
  outline: none;
  opacity: 0.8;
  cursor: pointer;
}

.editor-header > div > button:hover:not(:disabled) {
  opacity: 1;
}

.editor-header > div > button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.editor-header > div > button > span > i {
  font-size: 1.2rem;
}
</style>
