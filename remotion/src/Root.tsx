import React from 'react';
import {Composition} from 'remotion';
import {PodcastClip} from './PodcastClip';
import {defaultProps, editPropsSchema} from './schema';

export const COMPOSITION_ID = 'PodcastClip';

export const PodcastRoot: React.FC = () => (
  <Composition
    id={COMPOSITION_ID}
    component={PodcastClip}
    schema={editPropsSchema}
    defaultProps={defaultProps}
    durationInFrames={150}
    fps={30}
    width={1080}
    height={1920}
    calculateMetadata={({props}) => {
      // Props arrive from the Go backend as JSON. Parse (not just type-check)
      // so defaults are filled in and bad values fail early with a clear error.
      const parsed = editPropsSchema.parse(props);
      return {
        props: parsed,
        durationInFrames: Math.max(1, Math.ceil(parsed.duration * parsed.fps)),
        fps: parsed.fps,
        width: parsed.width,
        height: parsed.height,
      };
    }}
  />
);
