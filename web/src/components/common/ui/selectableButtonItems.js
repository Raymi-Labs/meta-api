// Keep selection and reset controls reachable when an opted-in filter has no results.
export const isSelectableItemDisabled = (item) =>
  Boolean(
    item.disabled || (typeof item.tagCount === 'number' && item.tagCount === 0),
  );

export const getSelectableButtonItems = (
  items,
  activeValue,
  hideDisabled = false,
  resetValue,
) => {
  if (!hideDisabled) return items;
  return items.filter(
    (item) =>
      item.value === resetValue ||
      (Array.isArray(activeValue)
        ? activeValue.includes(item.value)
        : item.value === activeValue) ||
      !isSelectableItemDisabled(item),
  );
};
