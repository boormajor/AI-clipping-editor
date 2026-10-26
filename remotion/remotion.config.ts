import {Config} from '@remotion/cli/config';

// Rendering defaults. The Go backend passes the output path, props file and
// codec explicitly; these only cover what is not set per job.
Config.setVideoImageFormat('jpeg');
Config.setOverwriteOutput(true);
Config.setChromiumOpenGlRenderer('angle');
