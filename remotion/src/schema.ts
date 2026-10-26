import {z} from 'zod';
import {zColor} from '@remotion/zod-types';

/**
 * Shared prop schema for the PodcastClip composition.
 *
 * The Go backend writes a props JSON file that is validated against this schema
 * before a frame is rendered. Every field has a default so a partial or stale
 * props file still produces a sensible render instead of a crash mid-job.
 */

export const CAPTION_PRESETS = [
  'kinetic',
  'editorial',
  'bold',
  'minimal',
  'karaoke',
  'boxed',
  'podcast',
  'yellow',
  'highlight',
  'popline',
  'wordbox',
  'wipe',
  'slide',
  'bounce',
  'typewriter',
] as const;
export type CaptionPreset = (typeof CAPTION_PRESETS)[number];

export const LAYOUTS = ['focus', 'two-up', 'dynamic'] as const;
export type Layout = (typeof LAYOUTS)[number];

export const SHOT_MODES = ['both', 'left', 'right', 'full'] as const;

export const wordSchema = z.object({
  start: z.number().min(0),
  end: z.number().min(0),
  word: z.string(),
});

export const cueSchema = z.object({
  start: z.number().min(0),
  end: z.number().min(0),
  text: z.string(),
});

export const shotSchema = z.object({
  start: z.number().min(0),
  end: z.number().min(0),
  mode: z.enum(SHOT_MODES),
  pan: z.number().min(0).max(1).optional(),
});

export const assetSchema = z.object({
  media: z.string(),
  start: z.number().min(0),
  duration: z.number().positive(),
  /** 'cover' fills the frame; 'pip' shows a rounded picture-in-picture tile. */
  mode: z.enum(['cover', 'pip']).default('cover'),
});

export const editPropsSchema = z.object({
  media: z.string().default(''),
  duration: z.number().positive().default(5),
  width: z.number().int().min(240).max(3840).default(1080),
  height: z.number().int().min(240).max(3840).default(1920),
  fps: z.number().int().min(1).max(120).default(30),
  layout: z.enum(LAYOUTS).default('focus'),
  /** 0 = left edge, 0.5 = centre, 1 = right edge of the source frame. */
  pan: z.number().min(0).max(1).default(0.5),
  shots: z.array(shotSchema).default([]),
  cues: z.array(cueSchema).default([]),
  words: z.array(wordSchema).default([]),
  assets: z.array(assetSchema).default([]),
  captions: z.boolean().default(true),
  captionPreset: z.enum(CAPTION_PRESETS).default('kinetic'),
  captionFont: z.string().default('Inter'),
  captionSize: z.number().min(24).max(150).default(76),
  captionColor: zColor().default('#FFFFFF'),
  accentColor: zColor().default('#FACC15'),
  captionOffset: z.number().min(-10).max(10).default(0),
  captionWords: z.number().int().min(1).max(12).default(3),
  /** Vertical anchor of the caption block as a fraction of the frame height. */
  captionPosition: z.number().min(0.1).max(0.95).default(0.82),
  title: z.string().default(''),
  callout: z.string().default(''),
  calloutSeconds: z.number().min(0).max(60).default(3.4),
  graphics: z.boolean().default(false),
  zoom: z.boolean().default(false),
  emphasis: z.boolean().default(false),
  /** Show a progress bar along the bottom edge of the clip. */
  progressBar: z.boolean().default(false),
});

export type EditProps = z.infer<typeof editPropsSchema>;
export type EditInput = z.input<typeof editPropsSchema>;
export type Word = z.infer<typeof wordSchema>;
export type Cue = z.infer<typeof cueSchema>;
export type Shot = z.infer<typeof shotSchema>;
export type Asset = z.infer<typeof assetSchema>;

export const defaultProps: EditProps = editPropsSchema.parse({});
