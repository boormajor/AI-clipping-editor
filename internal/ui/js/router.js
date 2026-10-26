import {set} from './store.js';

const ROUTES = ['import', 'clips', 'settings', 'connections', 'queue'];

export function navigate(route) {
  if (!ROUTES.includes(route)) route = 'import';
  if (location.hash !== '#/' + route) location.hash = '#/' + route;
  else render(route);
}

function render(route) {
  document.querySelectorAll('.page').forEach((p) => p.classList.toggle('visible', p.id === 'page-' + route));
  document.querySelectorAll('.nav[data-route]').forEach((b) => {
    if (b.dataset.route === route) b.setAttribute('aria-current', 'page');
    else b.removeAttribute('aria-current');
  });
  set({route});
  const heading = document.querySelector(`#page-${route} h1`);
  if (heading) {
    heading.setAttribute('tabindex', '-1');
    heading.focus({preventScroll: true});
  }
  document.getElementById('main').scrollTop = 0;
}

export function initRouter(defaultRoute) {
  const fromHash = () => render(location.hash.replace(/^#\//, '') || defaultRoute);
  window.addEventListener('hashchange', fromHash);
  document.querySelectorAll('.nav[data-route]').forEach((b) => b.addEventListener('click', () => navigate(b.dataset.route)));
  fromHash();
}
