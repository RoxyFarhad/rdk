<script lang="ts">

import { grpc } from '@improbable-eng/grpc-web';
import { Client, gripperApi } from '@viamrobotics/sdk';
import { displayError } from '../../lib/error';
import { rcLogConditionally } from '../../lib/log';
import Collapse from '@/components/collapse.svelte';

export let name: string;
export let client: Client;

const stop = () => {
  const request = new gripperApi.StopRequest();
  request.setName(name);

  rcLogConditionally(request);
  client.gripperService.stop(request, new grpc.Metadata(), displayError);
};

const open = () => {
  const request = new gripperApi.OpenRequest();
  request.setName(name);

  rcLogConditionally(request);
  client.gripperService.open(request, new grpc.Metadata(), displayError);
};

const grab = () => {
  const request = new gripperApi.GrabRequest();
  request.setName(name);

  rcLogConditionally(request);
  client.gripperService.grab(request, new grpc.Metadata(), displayError);
};

</script>

<Collapse title={name}>
  <v-breadcrumbs
    slot="title"
    crumbs="gripper"
  />
  <div
    slot="header"
    class="flex items-center justify-between gap-2"
  >
    <v-button
      variant="danger"
      icon="stop-circle"
      label="Stop"
      on:click|stopPropagation={stop}
    />
  </div>
  <div class="flex gap-2 border border-t-0 border-medium p-4">
    <v-button
      label="Open"
      on:click={open}
    />
    <v-button
      label="Grab"
      on:click={grab}
    />
  </div>
</Collapse>
