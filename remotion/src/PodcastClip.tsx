import React from 'react';
import {AbsoluteFill, Audio, staticFile} from 'remotion';
import type {EditProps} from './schema';
import {CaptionLayer} from './captions';
import {BrollLayer, GraphicsLayer, ProgressBar, VideoStage} from './stage';

/**
 * The full composition: framed source video, B-roll overlays, a callout
 * banner, captions and an optional progress bar.
 *
 * Dialogue audio is emitted once from a dedicated <Audio> element; every
 * visual panel is muted so stacked layouts never double the soundtrack.
 */
export const PodcastClip: React.FC<EditProps> = (p) => {
  if (!p.media) {
    return (
      <AbsoluteFill style={{background: '#090A12', color: '#8fa3c7', justifyContent: 'center', alignItems: 'center', fontFamily: 'Inter, Arial, sans-serif', fontSize: 40, textAlign: 'center', padding: 80}}>
        No source media. Render this composition through Podcast AI Studio, or pass a props file with a `media` path under public/.
      </AbsoluteFill>
    );
  }
  return (
    <AbsoluteFill style={{overflow: 'hidden', background: '#090A12'}}>
      <VideoStage p={p} />
      <Audio src={staticFile(p.media)} />
      <BrollLayer p={p} />
      <GraphicsLayer p={p} />
      <CaptionLayer p={p} />
      <ProgressBar p={p} />
    </AbsoluteFill>
  );
};
