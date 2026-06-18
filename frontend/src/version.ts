declare const __APP_VERSION__: string;

export const VERSION = typeof __APP_VERSION__ === "string" ? __APP_VERSION__ : "dev";
